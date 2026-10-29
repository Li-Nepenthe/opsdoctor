// 本文件：用可控时间和阻塞模型验证调度名额、过时任务、费用拦截与取消。
//
// 测试不靠睡几毫秒猜 goroutine 是否运行了。synctest 等待所有任务确实阻塞后，
// 再检查运行名单；模型只等 context，不连接网络。
//
// 主要内容：TestTransitions 穷举状态边界；TestSchedulerIsolation 检查两个系统；
// TestSchedulerBudgetAndExpiry 检查额度、旧任务和启动清理；TestCancelBeforeStart 检查取消空窗。
// TestBudgetStatusReason 检查持久账本原因优先于进程内预留失败标志。
// TestRenewUnavailable 检查数据库一直断开时，执行者会在旧租约到期前停工。
// 阅读建议：假仓储只模拟状态竞争，SQL 原子性另由 store 集成测试证明。

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// queued 保存假的持久化状态，所有访问由 memoryTasks.mu 保护。
type queued struct {
	task   Task
	status string
	reason string
	alert  bool
	stale  bool
	seq    int
}

// memoryTasks 的开始开关专门复现“已登记在途、尚未写 running”这个取消窗口。
type memoryTasks struct {
	mu         sync.Mutex
	items      []*queued
	blockStart bool
	recovered  bool
}

// Recover 模拟过时断点的清理；仍在恢复期限内的任务由后续调度接手。
func (memory *memoryTasks) Recover(context.Context) error {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	memory.recovered = true
	for _, item := range memory.items {
		if item.status == Running && item.stale {
			item.status, item.reason = Failed, "中断时间超过恢复期限，旧证据已过时，未继续诊断"
		}
	}
	return nil
}

// Expire 只改旧告警，手动排队任务不受影响。
func (memory *memoryTasks) Expire(_ context.Context, cutoff time.Time) error {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	for _, item := range memory.items {
		if item.alert && item.status == Pending && item.task.CreatedAt.Before(cutoff) {
			item.status, item.reason = Cancelled, "告警已过时，未执行"
		}
	}
	return nil
}

// Pending 保留提交顺序，测试会把五个 A 都放到 B 前面，刻意制造抢占机会。
func (memory *memoryTasks) Pending(context.Context) ([]Task, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	var tasks []Task
	for _, item := range memory.items {
		if item.status == Pending {
			tasks = append(tasks, item.task)
		}
	}
	return tasks, nil
}

// StartTask 模拟状态的条件更新，只有 pending 才能开始。
func (memory *memoryTasks) StartTask(ctx context.Context, id, _, _, _ string) (bool, error) {
	if memory.blockStart {
		<-ctx.Done()
		return false, ctx.Err()
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	for _, item := range memory.items {
		if item.task.ID == id && item.status == Pending {
			item.status = Running
			return true, nil
		}
	}
	return false, nil
}

// EndTask 不覆盖已有终态，让迟到结果也经过同一条件约束。
func (memory *memoryTasks) EndTask(_ context.Context, id, status string, _ agent.Result, reason string) error {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	for _, item := range memory.items {
		if item.task.ID == id && item.status == Running {
			item.status, item.reason = status, reason
		}
	}
	return nil
}

// CancelPending 与 StartTask 竞争同一个状态，只有一方能先推进。
func (memory *memoryTasks) CancelPending(_ context.Context, id, reason string) (bool, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	for _, item := range memory.items {
		if item.task.ID == id && item.status == Pending {
			item.status, item.reason = Cancelled, reason
			return true, nil
		}
	}
	return false, nil
}

// RecordStep 分配测试中的真实步骤号，Agent 仍要先保存才能引用。
func (memory *memoryTasks) RecordStep(_ context.Context, id string, step agent.Step) (agent.Step, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	for _, item := range memory.items {
		if item.task.ID == id {
			item.seq++
			step.Seq = item.seq
			return step, nil
		}
	}
	return step, fmt.Errorf("任务不存在")
}

// blockingModel 在正常调用中不返回，测试取消必须一路传到这个 context。
type blockingModel struct{}

// Complete 不访问供应商，只模拟等待中的 HTTP 请求。
func (blockingModel) Complete(ctx context.Context, _ llm.Request) (llm.Response, error) {
	<-ctx.Done()
	return llm.Response{}, ctx.Err()
}

// testScheduler 使用真实 Agent 与假模型，避免仅测试一个与生产不同的执行分支。
func testScheduler(t *testing.T, memory *memoryTasks, provider string, blocked bool) *Scheduler {
	t.Helper()
	loop, err := agent.New(blockingModel{}, "mock-test", llm.NewRedactor(), agent.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	scheduler, err := New(Config{Model: "mock-test", Provider: provider, Concurrent: 2, PerTarget: 1, PendingAge: 5 * time.Minute, Budget: func(context.Context) (BudgetStatus, error) {
		return BudgetStatus{LimitMicros: 20_000_000, Blocked: blocked}, nil
	}, Resolve: func(string) (agent.Input, error) {
		return agent.Input{Profile: "虚构架构", Components: []string{"unknown", "api"}, Tools: tools.New(tools.Dependencies{Profile: "虚构架构"})}, nil
	}}, memory, loop, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return scheduler
}

// taskForTest 生成足够新的任务，只有需要过期的用例才主动把时间往前移动。
func taskForTest(id, target string) *queued {
	return &queued{task: Task{ID: id, Target: target, Input: json.RawMessage(`{}`), CreatedAt: time.Now()}, status: Pending, alert: true}
}

// TestTransitions 穷举全部状态对；终态回到 running 等情况必须拒绝。
func TestTransitions(t *testing.T) {
	values := []string{Pending, Running, WaitingApproval, Succeeded, Failed, Cancelled, BudgetExceeded, "invented"}
	for _, from := range values {
		for _, to := range values {
			allowed := from == Pending && (to == Running || to == Cancelled) || from == Running && (to == WaitingApproval || to == Succeeded || to == Failed || to == Cancelled || to == BudgetExceeded) || from == WaitingApproval && (to == Running || to == Failed || to == Cancelled)
			if got := Transition(from, to) == nil; got != allowed {
				t.Fatalf("%s -> %s: got=%v", from, to, got)
			}
		}
	}
}

// TestSchedulerIsolation 五个 A 只占一个名额，B 同时运行；取消后全部 worker 都退出。
func TestSchedulerIsolation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		memory := &memoryTasks{}
		for i := range 5 {
			memory.items = append(memory.items, taskForTest(fmt.Sprint(i), "A"))
		}
		memory.items = append(memory.items, taskForTest("B", "B"))
		scheduler := testScheduler(t, memory, "mock", false)
		if err := scheduler.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		// Wait 保证所有任务确实进入阻塞，不依赖机器调度快慢。
		memory.mu.Lock()
		counts := map[string]int{}
		for _, item := range memory.items {
			if item.status == Running {
				counts[item.task.Target]++
			}
		}
		memory.mu.Unlock()
		if counts["A"] != 1 || counts["B"] != 1 {
			t.Fatalf("并发=%v", counts)
		}
		started := time.Now()
		scheduler.Close()
		if time.Since(started) > 2*time.Second {
			t.Fatal("关闭超过两秒")
		}
		memory.mu.Lock()
		defer memory.mu.Unlock()
		if memory.items[0].status != Running || memory.items[5].status != Running {
			t.Fatal("关闭进程应保留 running 断点供新进程接手")
		}
	})
}

// TestSchedulerBudgetAndExpiry 额度已满也要清理过时任务，而且不能启动任意付费调用。
func TestSchedulerBudgetAndExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		old, fresh, orphan := taskForTest("old", "A"), taskForTest("fresh", "B"), taskForTest("orphan", "C")
		old.task.CreatedAt = time.Now().Add(-6 * time.Minute)
		orphan.status, orphan.stale = Running, true
		memory := &memoryTasks{items: []*queued{old, fresh, orphan}}
		scheduler := testScheduler(t, memory, "openai", true)
		if err := scheduler.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		scheduler.Close()
		// 新任务保留 pending，旧告警与重启中断各有自己的明确原因。
		if fresh.status != Pending || old.status != Cancelled || old.reason != "告警已过时，未执行" || orphan.status != Failed || orphan.reason != "中断时间超过恢复期限，旧证据已过时，未继续诊断" {
			t.Fatal("启动清理或费用拦截错误")
		}
	})
}

// TestCancelBeforeStart 取消已登记但尚未开始的任务，不能等下次轮询又把它拿起来执行。
func TestCancelBeforeStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// 把 StartTask 卡住，取消必须处理仍是 pending 的数据库行。
		item := taskForTest("A", "A")
		memory := &memoryTasks{items: []*queued{item}, blockStart: true}
		scheduler := testScheduler(t, memory, "mock", false)
		if err := scheduler.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if err := scheduler.Cancel(t.Context(), "A"); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		scheduler.Close()
		if item.status != Cancelled {
			t.Fatalf("启动前取消状态=%s", item.status)
		}
	})
}

// TestBudgetStatusReason 防止一次预留失败盖掉更需要人工核对的持久停用原因。
func TestBudgetStatusReason(t *testing.T) {
	// 分别覆盖“账本已有更具体原因”和“只有当前进程知道预留不足”两种来源。
	for _, test := range []struct{ name, reason, code, want string }{
		{"保留记账保护", "实际费用超出预留，请人工核对", "accounting_guard", "accounting_guard"},
		{"没有持久原因时解释预留不足", "", "", "request_reservation"},
	} {
		t.Run(test.name, func(t *testing.T) {
			scheduler := &Scheduler{blocked: true, config: Config{Budget: func(context.Context) (BudgetStatus, error) {
				return BudgetStatus{Reason: test.reason, ReasonCode: test.code}, nil
			}}}
			got, err := scheduler.Status(t.Context())
			if err != nil || !got.Blocked || got.ReasonCode != test.want || got.Reason == "" {
				t.Fatalf("状态=%+v err=%v", got, err)
			}
			if test.reason != "" && got.Reason != test.reason {
				t.Fatal("持久原因被覆盖")
			}
		})
	}
}

// Renew 在单进程假仓储中确认任务还在运行；租约时钟和多进程竞争由真实库测试覆盖。
func (memory *memoryTasks) Renew(_ context.Context, id string) (bool, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	for _, item := range memory.items {
		if item.task.ID == id {
			return item.status == Running, nil
		}
	}
	return false, nil
}

// Resume 的这些调度测试没有历史；Agent 对话回放另用带步骤的专门用例。
func (memory *memoryTasks) Resume(context.Context, string) ([]agent.Step, time.Duration, int64, error) {
	return nil, 0, 0, nil
}

// unavailableTasks 模拟续租请求一直失败，其他行为沿用内存仓储。
type unavailableTasks struct{ *memoryTasks }

// Renew 返回网络错误，不能用它证明旧租约仍然有效。
func (unavailableTasks) Renew(context.Context, string) (bool, error) {
	return false, fmt.Errorf("测试：数据库不可达")
}

// TestRenewUnavailable 用虚拟时钟等到租约边界，避免依赖真实机器的调度速度。
func TestRenewUnavailable(t *testing.T) {
	// 用虚拟时间分别观察到期前与到期后，真实调度快慢不会改变结论。
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		scheduler := &Scheduler{
			config:     Config{LeaseTTL: 30 * time.Second},
			repository: unavailableTasks{&memoryTasks{}},
			running:    map[string]running{"lost": {cancel: cancel}},
			logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		go scheduler.renew(ctx, "lost")
		synctest.Wait()
		time.Sleep(29 * time.Second)
		synctest.Wait()
		if ctx.Err() != nil {
			t.Fatal("短暂失败不应提前耗掉旧租约")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if ctx.Err() != context.Canceled {
			t.Fatal("数据库一直不可达时，旧租约到期必须停工")
		}
	})
}
