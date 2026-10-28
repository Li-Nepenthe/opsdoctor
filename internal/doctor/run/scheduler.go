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
	ID        string          // 数据库任务 ID。
	Target    string          // 创建时已绑定的系统。
	Input     json.RawMessage // 脱敏后的告警或手动症状。
	CreatedAt time.Time       // 过时检查依据。
}

// BudgetStatus 同时供调度器、页面和指标读取，金额是百万分之一元。
type BudgetStatus struct {
	SpentMicros int64  `json:"spent_micros"` // 含无法确认扣费时保留的预留额。
	LimitMicros int64  `json:"limit_micros"` // 本进程配置的总上限。
	Blocked     bool   `json:"blocked"`      // true 时真实模型不再启动新任务。
	Reason      string `json:"reason"`       // 可向用户解释的固定原因。
}

// Repository 把状态与步骤保存放在同一个边界，假仓储可完整验证调度行为。
type Repository interface {
	Recover(context.Context) error
	Expire(context.Context, time.Time) error
	Pending(context.Context) ([]Task, error)
	StartTask(context.Context, string, string, string, string) (bool, error)
	EndTask(context.Context, string, string, agent.Result, string) error
	CancelPending(context.Context, string, string) (bool, error)
	RecordStep(context.Context, string, agent.Step) (agent.Step, error)
}

// Config 保存进程内调度参数；Resolve 在任务开始时返回已固定系统的工具与说明。
type Config struct {
	Model          string                                      // 初始模型名写入 diagnoses.model。
	Provider       string                                      // mock 或 openai，决定是否检查真实费用。
	Concurrent     int                                         // 全局名额，默认 2，范围 1–2。
	PerTarget      int                                         // 同系统名额，本阶段固定 1。
	PendingAge     time.Duration                               // 告警 pending 超过多久不再执行。
	Resolve        func(string) (agent.Input, error)           // 来自目标目录，不接触标准答案。
	Budget         func(context.Context) (BudgetStatus, error) // 真实账本快照。
	Published      func(string, agent.Step)                    // 已入库后才推实时通知和指标。
	ObservedBudget func(BudgetStatus)                          // 把同一份额度状态同步到指标，不额外查库。
}

// running 保存取消函数和系统名，mu 保护这张表与 stopped、blocked。
type running struct {
	target string
	cancel context.CancelFunc
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
	if repository == nil || agent == nil || logger == nil || config.Resolve == nil || config.Budget == nil || config.Concurrent < 1 || config.Concurrent > 2 || config.PerTarget != 1 || config.PendingAge <= 0 || (config.Provider != "mock" && config.Provider != "openai") {
		return nil, fmt.Errorf("调度依赖或并发配置不合法")
	}
	return &Scheduler{config: config, repository: repository, agent: agent, logger: logger, running: map[string]running{}, wake: make(chan struct{}, 1)}, nil
}

// Start 先失败化遗留 running，再清理过时告警；任何一步失败都不启动后台诊断。
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
		work, cancel := context.WithCancel(ctx)
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
		// 取消可能恰好发生在登记在途表之后、写 running 之前。此时也必须留下取消结果。
		if ctx.Err() != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 500*time.Millisecond)
			defer cancel()
			changed, saveErr := scheduler.repository.CancelPending(cleanup, task.ID, "用户取消或服务停止")
			if saveErr == nil && !changed {
				saveErr = scheduler.repository.EndTask(cleanup, task.ID, Cancelled, agent.Result{}, "用户取消或服务停止")
			}
			if saveErr != nil {
				scheduler.logger.Error("保存启动前取消失败", "diagnosis_id", task.ID, "error", saveErr)
			}
		} else if err != nil {
			scheduler.logger.Warn("开始诊断失败", "diagnosis_id", task.ID, "error", err)
		}
		return
	}
	input, err := scheduler.config.Resolve(task.Target)
	var result agent.Result
	if err == nil {
		input.ID, input.Symptoms, input.Recorder = task.ID, string(task.Input), taskRecorder{scheduler: scheduler, id: task.ID}
		result, err = scheduler.agent.Run(ctx, input)
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
	case err != nil:
		status, reason = Failed, err.Error()
	case result.BudgetExceeded:
		status, reason = BudgetExceeded, "查证预算已用尽，最后一次机会未提交合法报告"
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 500*time.Millisecond)
	defer cancel()
	if saveErr := scheduler.repository.EndTask(cleanup, task.ID, status, result, reason); saveErr != nil {
		scheduler.logger.Error("保存诊断结束状态失败", "diagnosis_id", task.ID, "error", saveErr)
		return
	}
	scheduler.logger.Info("诊断结束", "diagnosis_id", task.ID, "target", task.Target, "status", status, "rounds", result.Rounds, "tool_calls", result.ToolCalls)
}

// Cancel 取消正在运行的 context；排队任务直接以条件更新改为 cancelled。
func (scheduler *Scheduler) Cancel(ctx context.Context, id string) error {
	scheduler.mu.Lock()
	// 登记在途比写 running 更早，所以取消总能找到执行中的 context。
	current, exists := scheduler.running[id]
	if exists {
		current.cancel()
	}
	scheduler.mu.Unlock()
	if exists {
		return nil
	}
	changed, err := scheduler.repository.CancelPending(ctx, id, "用户取消")
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("任务已结束或不属于当前进程的在途任务")
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
		status.Reason = llm.ErrBudget.Error()
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
	if err == nil && recorder.scheduler.config.Published != nil {
		recorder.scheduler.config.Published(recorder.id, saved)
	}
	return saved, err
}
