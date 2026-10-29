// 本文件：为排队诊断分配名额，运行结束后回收名额，并处理取消。
//
// 一个系统的任务很多时，仍要给其他系统留机会。仓储按系统返回最早的候选，
// 调度器再检查全局和系统名额。执行与 API 取消共用一张受锁保护的在途表。
//
// 主要内容：Repository 约定持久化；Start 清理并启动循环；dispatch 分配名额；execute 执行任务。
// 阅读建议：在途记录先登记再改数据库状态，取消请求不会落在两者之间的空窗。

package run

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// Task 只包含执行所需的最小字段，模型不能改变 ID 或 Target。
type Task struct {
	FaultStartedAt  time.Time       // 初始故障时间；查询不能默认混入更早的一场实验。
	EvaluationGroup string          // 空值是普通诊断，E0–E3 是由评测器固定的工具组。
	ExpectedModel   string          // 评测指定的模型；切换进程后不能悄悄用另一个模型续跑。
	ID              string          // 数据库任务 ID。
	Target          string          // 创建时已绑定的系统。
	Input           json.RawMessage // 脱敏后的告警或手动症状。
	CreatedAt       time.Time       // 过时检查依据。
}

// BudgetStatus 同时供调度器、页面和指标读取，金额是百万分之一账户货币单位。
type BudgetStatus struct {
	Currency    string `json:"currency"`              // 本账户币种，不能把历史人民币显示成美元。
	SpentMicros int64  `json:"spent_micros"`          // 含无法确认扣费时保留的预留额。
	LimitMicros int64  `json:"limit_micros"`          // 本进程配置的总上限。
	Blocked     bool   `json:"blocked"`               // true 时真实模型不再启动新任务。
	Reason      string `json:"reason"`                // 可向用户解释的固定原因。
	ReasonCode  string `json:"reason_code,omitempty"` // 有限分类供指标使用，不能拿任意错误文字做标签。
}

// Repository 把状态与步骤保存放在同一个边界，假仓储可完整验证调度行为。
type Repository interface {
	Recover(context.Context) error
	Renew(context.Context, string) (bool, error)
	Resume(context.Context, string) ([]agent.Step, time.Duration, int64, error)
	Expire(context.Context, time.Time) error
	Pending(context.Context) ([]Task, error)
	StartTask(context.Context, string, string, string, string) (bool, error)
	EndTask(context.Context, string, string, agent.Result, string) error
	CancelPending(context.Context, string, string) (bool, error)
	RecordStep(context.Context, string, agent.Step) (agent.Step, error)
}

// Config 保存进程内调度参数；Resolve 在任务开始时返回已固定系统的工具与说明。
type Config struct {
	LeaseTTL time.Duration // 续约间隔取三分之一，留出瞬时数据库失败的余量。

	Model          string                                      // 初始模型名写入 diagnoses.model。
	Provider       string                                      // mock 或 openai，决定是否检查真实费用。
	Concurrent     int                                         // 全局名额，默认 2，范围 1–2。
	PerTarget      int                                         // 同系统名额，本阶段固定 1。
	PendingAge     time.Duration                               // 告警 pending 超过多久不再执行。
	Resolve        func(string) (agent.Input, error)           // 来自目标目录，不接触标准答案。
	Prepare        func(Task, *agent.Input) error              // 按任务筛选工具；不接收场景答案。
	Budget         func(context.Context) (BudgetStatus, error) // 真实账本快照。
	Published      func(string, agent.Step)                    // 已入库后才推实时通知和指标。
	ObservedBudget func(BudgetStatus)                          // 把同一份额度状态同步到指标，不额外查库。
}

// running 保存取消函数和系统名，mu 保护这张表与 stopped、blocked。
type running struct {
	userCancelled bool // 显式用户操作，与停机和丢租约区分。
	target        string
	cancel        context.CancelFunc
}

// Scheduler 的 worker 都计入 WaitGroup，Close 等它们退出后才允许 app 关数据库。
type Scheduler struct {
	mu         sync.Mutex         // 保护 running、stopped、blocked。
	running    map[string]running // 仅包含本进程持有的执行，不用于恢复。
	stopped    bool               // 停机后禁止新增 worker。
	blocked    bool               // 本轮发现预留额不足时阻止后续真实任务。
	config     Config             // 构造后固定。
	repository Repository         // 实际数据库或测试实现。
	agent      *agent.Agent       // 无共享对话状态，可以并行 Run。
	logger     *slog.Logger       // 只记任务 ID 与状态，不记模型正文。
	wake       chan struct{}      // API 可唤醒，容量一合并重复信号；不关闭，随 loop 退出失效。
	workers    sync.WaitGroup     // loop 与每个执行 goroutine 各登记一次。
	cancel     context.CancelFunc // 关闭整个调度树。
}

// New 只检查配置，Start 完成数据库清理后才真正接受任务。
func New(config Config, repository Repository, agent *agent.Agent, logger *slog.Logger) (*Scheduler, error) {
	if config.LeaseTTL == 0 {
		config.LeaseTTL = 30 * time.Second
	}
	if repository == nil || agent == nil || logger == nil || config.Resolve == nil || config.Budget == nil || config.Concurrent < 1 || config.Concurrent > 2 || config.PerTarget != 1 || config.PendingAge <= 0 || (config.Provider != "mock" && config.Provider != "openai") {
		return nil, fmt.Errorf("调度依赖或并发配置不合法")
	}
	return &Scheduler{config: config, repository: repository, agent: agent, logger: logger, running: map[string]running{}, wake: make(chan struct{}, 1)}, nil
}

// Start 先检查过时的断点，再清理过时告警；可恢复的任务会由循环接手。
func (scheduler *Scheduler) Start(parent context.Context) error {
	if err := scheduler.repository.Recover(parent); err != nil {
		return err
	}
	if err := scheduler.repository.Expire(parent, time.Now().Add(-scheduler.config.PendingAge)); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	scheduler.cancel = cancel
	scheduler.workers.Add(1)
	// loop 由 parent 或 Close 取消；它退出后不会再向 WaitGroup 增加 worker。
	go func() { defer scheduler.workers.Done(); scheduler.loop(ctx) }()
	return nil
}

// Wake 合并高频通知，后台仍定期轮询，因此丢掉重复唤醒不会丢任务。
func (scheduler *Scheduler) Wake() {
	select {
	case scheduler.wake <- struct{}{}:
	default:
	}
}

// Close 先阻止分配新名额，再取消所有在途任务，最后等待 goroutine 退出。
func (scheduler *Scheduler) Close() {
	scheduler.mu.Lock()
	scheduler.stopped = true
	if scheduler.cancel != nil {
		scheduler.cancel()
	}
	for _, current := range scheduler.running {
		current.cancel()
	}
	scheduler.mu.Unlock()
	scheduler.workers.Wait()
}

// loop 每秒检查队列；通知只是加快响应，不承担持久化职责。
func (scheduler *Scheduler) loop(ctx context.Context) {
	const poll = time.Second // 队列写入后最多约一秒被发现，不高频扫描 MySQL。
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := scheduler.dispatch(ctx); err != nil {
			scheduler.logger.Warn("诊断调度本轮未完成", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-scheduler.wake:
		}
	}
}

// dispatch 清理过时任务后再看费用，这样额度用完也不会让旧告警永久卡在 pending。
func (scheduler *Scheduler) dispatch(ctx context.Context) error {
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := scheduler.repository.Recover(bounded); err != nil {
		return err
	}
	if err := scheduler.repository.Expire(bounded, time.Now().Add(-scheduler.config.PendingAge)); err != nil {
		return err
	}
	status, err := scheduler.Status(bounded)
	if err != nil {
		return err
	}
	if scheduler.config.Provider != "mock" && status.Blocked {
		return nil
	}
	tasks, err := scheduler.repository.Pending(bounded)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var nonce [16]byte
		_, _ = rand.Read(nonce[:])
		work, cancel := context.WithCancel(llm.WithLease(ctx, hex.EncodeToString(nonce[:])))
		if !scheduler.claim(task, cancel) {
			cancel()
			continue
		}
		scheduler.workers.Add(1)
		// execute 自己归还名额；work 由用户取消、停机或父 context 结束。
		go func() { defer scheduler.workers.Done(); defer cancel(); scheduler.execute(work, task) }()
	}
	return nil
}

// claim 在一把锁内同时检查两种名额，避免两条任务都看见“还有一个空位”。
func (scheduler *Scheduler) claim(task Task, cancel context.CancelFunc) bool {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if scheduler.stopped || len(scheduler.running) >= scheduler.config.Concurrent {
		return false
	}
	if _, exists := scheduler.running[task.ID]; exists {
		return false
	}
	for _, current := range scheduler.running {
		if current.target == task.Target {
			return false
		}
	}
	scheduler.running[task.ID] = running{target: task.Target, cancel: cancel}
	return true
}

// execute 将取消、预算和普通失败转换为有限状态；结束写入也有独立的短预算。
func (scheduler *Scheduler) execute(ctx context.Context, task Task) {
	defer func() {
		scheduler.mu.Lock()
		delete(scheduler.running, task.ID)
		scheduler.mu.Unlock()
		scheduler.Wake()
	}()
	started, err := scheduler.repository.StartTask(ctx, task.ID, scheduler.config.Model, agent.PromptVersion, scheduler.config.Provider)
	if err != nil || !started {
		if err != nil && ctx.Err() == nil {
			scheduler.logger.Warn("开始诊断失败", "diagnosis_id", task.ID, "error", err)
		}
		return
	}

	// time.Now 返回值带单调时钟读数，time.Since 不受系统校时影响。
	// 起止都留在本次 execute 中；恢复任务不能把另一进程的墙钟拼成耗时。
	executionStarted := time.Now()
	renewCtx, stopRenew := context.WithCancel(ctx)
	renewed := make(chan struct{})
	defer func() { stopRenew(); <-renewed }()
	go func() { defer close(renewed); scheduler.renew(renewCtx, task.ID) }()
	input, err := scheduler.config.Resolve(task.Target)
	if err == nil && scheduler.config.Prepare != nil {
		err = scheduler.config.Prepare(task, &input)
	}
	var result agent.Result
	if err == nil {
		input.ID, input.Symptoms, input.Recorder = task.ID, string(task.Input), taskRecorder{scheduler: scheduler, id: task.ID}
		input.History, input.ActiveTime, input.KnownCost, err = scheduler.repository.Resume(ctx, task.ID)
		if err == nil {
			result, err = scheduler.agent.Run(ctx, input)
		}
	}
	// 停机不是用户取消。保留断点等租约过期，下一进程才有机会继续。
	scheduler.mu.Lock()
	stopped := scheduler.stopped
	userCancelled := scheduler.running[task.ID].userCancelled
	scheduler.mu.Unlock()
	if stopped || (ctx.Err() != nil && !userCancelled) {
		return
	}
	status, reason := Succeeded, ""
	switch {
	case ctx.Err() != nil || errors.Is(err, context.Canceled):
		status, reason = Cancelled, "用户取消或服务停止"
	case errors.Is(err, llm.ErrBudget):
		status, reason = BudgetExceeded, llm.ErrBudget.Error()
		scheduler.mu.Lock()
		scheduler.blocked = true
		scheduler.mu.Unlock()
	case errors.Is(err, llm.ErrEvaluationBudget):
		status, reason = BudgetExceeded, err.Error()
	case err != nil:
		status, reason = Failed, err.Error()
	case result.WaitingApproval:
		status, reason = WaitingApproval, "等待人工审批，活动时间已暂停"
	case result.BudgetExceeded:
		status, reason = BudgetExceeded, "查证预算已用尽，最后一次机会未提交合法报告"
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 500*time.Millisecond)
	defer cancel()
	result.Elapsed = time.Since(executionStarted)
	if saveErr := scheduler.repository.EndTask(cleanup, task.ID, status, result, reason); saveErr != nil {
		scheduler.logger.Error("保存诊断结束状态失败", "diagnosis_id", task.ID, "error", saveErr)
		return
	}
	scheduler.logger.Info("诊断结束", "diagnosis_id", task.ID, "target", task.Target, "status", status, "rounds", result.Rounds, "tool_calls", result.ToolCalls)
}

// Cancel 取消正在运行的 context；排队任务直接以条件更新改为 cancelled。
func (scheduler *Scheduler) Cancel(ctx context.Context, id string) error {
	// 先持久化意图，点击取消之后即使进程马上崩溃，也不能在下次启动时重跑。
	changed, err := scheduler.repository.CancelPending(ctx, id, "用户取消")
	if err != nil {
		return err
	}
	scheduler.mu.Lock()
	current, exists := scheduler.running[id]
	if exists {
		current.userCancelled = true
		scheduler.running[id] = current
		current.cancel()
	}
	scheduler.mu.Unlock()
	if !changed && !exists {
		return fmt.Errorf("任务已结束或不存在")
	}
	return nil
}

// Status 同时显示已经花掉的额度和单次预留不足的原因，页面不能只看到“没有新任务”。
func (scheduler *Scheduler) Status(ctx context.Context) (BudgetStatus, error) {
	status, err := scheduler.config.Budget(ctx)
	scheduler.mu.Lock()
	blocked := scheduler.blocked
	scheduler.mu.Unlock()
	if blocked {
		status.Blocked = true
		// 账本已保存的停用原因更具体，不能被进程里的“预留失败”标志覆盖。
		if status.Reason == "" {
			status.Reason = llm.ErrBudget.Error()
			status.ReasonCode = "request_reservation"
		}
	}
	if err == nil && scheduler.config.ObservedBudget != nil {
		scheduler.config.ObservedBudget(status)
	}
	return status, err
}

// taskRecorder 把 agent 的一步绑定到任务 ID，并在提交后通知订阅者。
type taskRecorder struct {
	scheduler *Scheduler
	id        string
}

// Record 先保存后发布，SSE 的 Last-Event-ID 才一定能在数据库里找到。
func (recorder taskRecorder) Record(ctx context.Context, step agent.Step) (agent.Step, error) {
	saved, err := recorder.scheduler.repository.RecordStep(ctx, recorder.id, step)
	if err == nil && !saved.AlreadySaved && recorder.scheduler.config.Published != nil {
		recorder.scheduler.config.Published(recorder.id, saved)
	}
	return saved, err
}

// renew 定期证明执行者仍在工作；一次明确失去租约就取消本机请求，不能再继续花钱。
func (scheduler *Scheduler) renew(ctx context.Context, id string) {
	ticker := time.NewTicker(scheduler.config.LeaseTTL / 3)
	defer ticker.Stop()
	// 即使数据库一直不可达，也不能无限相信旧租约。用本机较早的起点保守停工。
	validUntil := time.Now().Add(scheduler.config.LeaseTTL)
	expiry := time.NewTimer(time.Until(validUntil))
	defer expiry.Stop()
	lose := func() {
		scheduler.mu.Lock()
		defer scheduler.mu.Unlock()
		if current, ok := scheduler.running[id]; ok {
			current.cancel()
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-expiry.C:
			lose()
			return
		case <-ticker.C:
			started := time.Now()
			deadline := minTime(started.Add(scheduler.config.LeaseTTL/6), validUntil)
			bounded, cancel := context.WithDeadline(ctx, deadline)
			ok, err := scheduler.repository.Renew(bounded, id)
			cancel()
			if err != nil {
				scheduler.logger.Warn("续租暂时失败", "diagnosis_id", id, "error", err)
				continue
			}
			if !ok {
				lose()
				return
			}
			// 从请求发出时起算，比收到成功回复后再加 TTL 更保守，避免网络耗时延长执行权。
			validUntil = started.Add(scheduler.config.LeaseTTL)
			expiry.Reset(max(0, time.Until(validUntil)))
		}
	}
}

// minTime 选较早的截止时间，续约查询本身也不能越过旧租约的保守期限。
func minTime(first, second time.Time) time.Time {
	if first.Before(second) {
		return first
	}
	return second
}
