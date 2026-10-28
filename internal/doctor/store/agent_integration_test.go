// 本文件：在专用数据库验证故障开放期、任务清理、步骤编号和累计费用。
//
// 这些断言依赖真实事务与行锁，内存替身不能证明它们。仍然只连医生测试库，
// 模型请求用内存数据代替，不访问供应商，也不产生费用。
//
// 主要内容：TestIntegrationFaultPeriod 检查一次故障；TestIntegrationTasks 检查状态；
// TestIntegrationBudget 检查并发预留与结算。
// 阅读建议：时间通过 Now 注入；测试跨过十五分钟不需要真的等待十五分钟。

package store

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/alerts"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
)

// TestIntegrationFaultPeriod 结束诊断不关闭故障；整组恢复和静默超时才允许新建。
func TestIntegrationFaultPeriod(t *testing.T) {
	repository, target := integrationStore(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	repository.config.Now = func() time.Time { return now }
	service := alerts.New(repository)
	notification := testNotification()
	first, err := service.Receive(t.Context(), target, notification)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repository.StartTask(t.Context(), first.DiagnosisID, "mock-test", "v-test", "mock"); err != nil {
		t.Fatal(err)
	}
	if err = repository.EndTask(t.Context(), first.DiagnosisID, run.Succeeded, agent.Result{}, ""); err != nil {
		t.Fatal(err)
	}
	// 同一故障新增一条告警，而且原诊断已经结束，仍复用原 ID。
	notification.Alerts = append(notification.Alerts, alerts.Alert{Status: "firing", Fingerprint: "fedcba9876543210", Labels: map[string]string{"alertname": "AnotherSymptom"}, StartsAt: now})
	for range 3 {
		got, err := service.Receive(t.Context(), target, notification)
		if err != nil || got.Created || got.DiagnosisID != first.DiagnosisID {
			t.Fatalf("重复=%+v err=%v", got, err)
		}
	}
	now = now.Add(time.Minute)
	notification.Status = "resolved"
	for i := range notification.Alerts {
		notification.Alerts[i].Status = "resolved"
		notification.Alerts[i].EndsAt = now
	}
	if _, err = service.Receive(t.Context(), target, notification); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	notification = testNotification()
	second, err := service.Receive(t.Context(), target, notification)
	if err != nil || !second.Created || second.DiagnosisID == first.DiagnosisID {
		t.Fatalf("新故障=%+v err=%v", second, err)
	}
	// 模拟恢复通知丢失，超过静默期限的下一次 firing 应创建第三轮。
	now = now.Add(16 * time.Minute)
	third, err := service.Receive(t.Context(), target, notification)
	if err != nil || !third.Created || third.DiagnosisID == second.DiagnosisID {
		t.Fatalf("静默兜底=%+v err=%v", third, err)
	}
}

// TestIntegrationTasks 证明并发步骤连续、取消不能被完成覆盖、启动清理留下明确原因。
func TestIntegrationTasks(t *testing.T) {
	repository, target := integrationStore(t)
	id, err := repository.Manual(t.Context(), target, "password=demo-hidden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repository.StartTask(t.Context(), id, "mock-test", "v-test", "mock"); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 12 {
		workers.Go(func() {
			if _, err := repository.RecordStep(t.Context(), id, agent.Step{Kind: "tool", Status: "ok", Result: "{}"}); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	steps, err := repository.StepsAfter(t.Context(), id, 0)
	if err != nil || len(steps) != 13 {
		t.Fatalf("步骤=%d err=%v", len(steps), err)
	}
	for i, step := range steps {
		if step.Seq != i+1 {
			t.Fatalf("序号断裂：%d", step.Seq)
		}
	}
	if err = repository.Recover(t.Context()); err != nil {
		t.Fatal(err)
	}
	detail, err := repository.Detail(t.Context(), id)
	if err != nil || detail.Status != run.Failed || detail.Error != "进程重启，恢复功能在阶段 7" {
		t.Fatalf("重启=%+v err=%v", detail, err)
	}
	// 恢复后的终态不能被迟到的 Agent 成功结果覆盖。
	if err = repository.EndTask(t.Context(), id, run.Succeeded, agent.Result{}, ""); err != nil {
		t.Fatal(err)
	}
	detail, _ = repository.Detail(t.Context(), id)
	if detail.Status != run.Failed {
		t.Fatal("终态被覆盖")
	}
	receipt, err := repository.ReceiveAlerts(t.Context(), target, testNotification())
	if err != nil {
		t.Fatal(err)
	}
	if err = repository.Expire(t.Context(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	detail, _ = repository.Detail(t.Context(), receipt.DiagnosisID)
	if detail.Status != run.Cancelled || detail.Error != "告警已过时，未执行" {
		t.Fatal("过时告警未取消")
	}
}

// TestIntegrationBudget 并发十次只能预留三次；失败回滚不能把总额冲到上限之外。
func TestIntegrationBudget(t *testing.T) {
	repository, target := integrationStore(t)
	id, err := repository.Manual(t.Context(), target, "预算集成测试")
	if err != nil {
		t.Fatal(err)
	}
	account := "test-" + newID()
	ledger, err := repository.NewLedger(t.Context(), account, 100)
	if err != nil {
		t.Fatal(err)
	}
	// 先清理尝试才能删账户；测试辅助的后续清理仍按系统删除诊断。
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = repository.database.ExecContext(ctx, "DELETE FROM llm_attempts WHERE account=?", account)
		_, _ = repository.database.ExecContext(ctx, "DELETE FROM llm_budget WHERE account=?", account)
	})
	var workers sync.WaitGroup
	var accepted atomic.Int32
	var mutex sync.Mutex
	var ids []string
	for range 10 {
		workers.Go(func() {
			attempt, err := ledger.Reserve(t.Context(), llm.Reservation{DiagnosisID: id, Model: "mock-test", Request: json.RawMessage(`{"messages":[]}`), MaximumMicros: 30})
			if err == nil {
				accepted.Add(1)
				mutex.Lock()
				ids = append(ids, attempt)
				mutex.Unlock()
			} else if !errors.Is(err, llm.ErrBudget) {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	status, err := ledger.Status(t.Context())
	if err != nil || accepted.Load() != 3 || status.SpentMicros != 90 {
		t.Fatalf("预留次数=%d 状态=%+v err=%v", accepted.Load(), status, err)
	}
	// 已知用量退回差额；重放结算不能重复退。未知用量保留整份预留。
	for range 2 {
		if err = ledger.Settle(t.Context(), ids[0], llm.Settlement{CostMicros: 7, Status: "ok"}); err != nil {
			t.Fatal(err)
		}
	}
	if err = ledger.Settle(t.Context(), ids[1], llm.Settlement{Unknown: true, Status: "cancelled"}); err != nil {
		t.Fatal(err)
	}
	status, _ = ledger.Status(t.Context())
	if status.SpentMicros != 67 {
		t.Fatalf("结算额=%d", status.SpentMicros)
	}
	reopened, err := repository.NewLedger(t.Context(), account, 60)
	if err != nil {
		t.Fatal(err)
	}
	status, _ = reopened.Status(t.Context())
	if !status.Blocked || status.SpentMicros != 67 {
		t.Fatal("重启或修改上限重置了账本")
	}
	// 供应商计量超过保守预留时也要记录实额，并持久化停用。即使重启后提高
	// 配置上限，也不能自动忽略这次估算失准，必须先人工核对供应商账单。
	if err = ledger.Settle(t.Context(), ids[2], llm.Settlement{CostMicros: 35, Status: "ok"}); !errors.Is(err, llm.ErrBudget) {
		t.Fatalf("超过预留额未停止：%v", err)
	}
	reopened, err = repository.NewLedger(t.Context(), account, 1000)
	if err != nil {
		t.Fatal(err)
	}
	status, _ = reopened.Status(t.Context())
	if !status.Blocked || status.SpentMicros != 72 || status.Reason == "" {
		t.Fatalf("停用未持久化：%+v", status)
	}
	_, err = reopened.Reserve(t.Context(), llm.Reservation{DiagnosisID: id, Model: "mock-test", Request: json.RawMessage(`{}`), MaximumMicros: 1})
	if !errors.Is(err, llm.ErrBudget) {
		t.Fatal("被停用的账户仍能发送下一次请求")
	}
}
