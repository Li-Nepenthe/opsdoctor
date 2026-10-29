// 本文件：在专用数据库模拟两个执行者争抢租约，以及模型回复保存一半时崩溃。
//
// 不真的等待租约过期，而是在测试专用任务上移动数据库时间字段。所有模型回复由
// 测试构造；这样能证明事务边界和费用保留，不会产生付费调用。
//
// 主要内容：TestIntegrationLeaseTakeover 检查续租、接手和旧写入；
// TestIntegrationModelRecovery 检查已结算回复回放与未知费用保留。
// 阅读建议：测试只改本次随机任务，清理由 integrationStore 按系统完成。

package store

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// TestIntegrationLeaseTakeover 活租约不能被抢，接手之后旧持有者的步骤和费用预留都被拒绝。
func TestIntegrationLeaseTakeover(t *testing.T) {
	repository, target := integrationStore(t)
	id, err := repository.Manual(t.Context(), target, "只用于租约测试")
	if err != nil {
		t.Fatal(err)
	}
	// 两个不同编号代表两个进程；只有旧租约到期，第二个才能接手。
	old, next := llm.WithLease(t.Context(), "old-owner"), llm.WithLease(t.Context(), "new-owner")
	if ok, err := repository.StartTask(old, id, "mock-old", "v-test", "mock"); err != nil || !ok {
		t.Fatalf("首次接手=%v %v", ok, err)
	}
	if ok, err := repository.StartTask(next, id, "mock-new", "v-test", "mock"); err != nil || ok {
		t.Fatal("抢到了活租约")
	}
	if err := repository.Recover(t.Context()); err != nil {
		t.Fatal(err)
	}
	if ok, err := repository.Renew(old, id); err != nil || !ok {
		t.Fatalf("活租约被启动清理结束：%v", err)
	}
	if _, err = repository.database.ExecContext(t.Context(), "UPDATE diagnoses SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND,lease_updated_at=UTC_TIMESTAMP(6)-INTERVAL 10 SECOND WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	if ok, _ := repository.Renew(old, id); ok {
		t.Fatal("过期租约被旧持有者续活")
	}
	if ok, err := repository.StartTask(next, id, "mock-new", "v-test", "mock"); err != nil || !ok {
		t.Fatalf("无法接手=%v %v", ok, err)
	}
	if _, err := repository.RecordStep(old, id, agent.Step{Kind: "tool", Status: "ok"}); err == nil {
		t.Fatal("旧持有者仍能保存步骤")
	}
	step := agent.Step{Kind: "tool", EventKey: "one-result", Status: "ok", Result: "{}"}
	first, err := repository.RecordStep(next, id, step)
	if err != nil {
		t.Fatal(err)
	}
	again, err := repository.RecordStep(next, id, step)
	if err != nil || again.Seq != first.Seq {
		t.Fatal("重试写了第二个步骤")
	}
	detail, err := repository.Detail(t.Context(), id)
	if err != nil || detail.Model != "mock-old" {
		t.Fatal("诊断开始模型被覆盖")
	}
	_, active, _, err := repository.Resume(next, id)
	if err != nil || active.Milliseconds() < 8000 {
		t.Fatalf("执行时间被重置：%v %v", active, err)
	}
}

// TestIntegrationModelRecovery 分别模拟成功结算后丢失步骤，以及崩溃时还不知道请求是否扣费。
func TestIntegrationModelRecovery(t *testing.T) {
	for _, known := range []bool{true, false} {
		t.Run(fmt.Sprintf("有成功回复=%v", known), func(t *testing.T) {
			repository, target := integrationStore(t)
			id, err := repository.Manual(t.Context(), target, "模型恢复测试")
			if err != nil {
				t.Fatal(err)
			}
			ctx := llm.WithLease(t.Context(), "test-lease")
			if ok, err := repository.StartTask(ctx, id, "mock-old", "v-test", "mock"); err != nil || !ok {
				t.Fatal(err)
			}
			request := json.RawMessage(`{"model":"mock-old","messages":[{"role":"system","content":"测试"}]}`)
			intent, err := repository.RecordStep(ctx, id, agent.Step{Kind: "system", Status: "llm_pending", Args: request})
			if err != nil {
				t.Fatal(err)
			}
			key := fmt.Sprintf("request:%d", intent.Seq)
			ledger, err := repository.NewLedger(t.Context(), "recovery-"+id, 100)
			if err != nil {
				t.Fatal(err)
			}
			// 账号行没有外键指回任务，须在测试专用记录清理完后单独回收。
			defer func() {
				_, _ = repository.database.ExecContext(t.Context(), "DELETE FROM llm_attempts WHERE diagnosis_id=?", id)
				_, _ = repository.database.ExecContext(t.Context(), "DELETE FROM llm_budget WHERE account=?", "recovery-"+id)
			}()
			attempt, err := ledger.Reserve(ctx, llm.Reservation{CallID: key, DiagnosisID: id, Model: "mock-old", Request: request, MaximumMicros: 50, DiagnosisLimitMicros: 100})
			if err != nil {
				t.Fatal(err)
			}
			if known {
				err = ledger.Settle(ctx, attempt, llm.Settlement{Status: "ok", CostMicros: 12, Usage: llm.Usage{Input: 3, Output: 2}, Response: json.RawMessage(`{"model":"actual-old","usage":{"prompt_tokens":3,"completion_tokens":2},"choices":[{"message":{"role":"assistant","content":"已保存回答"}}]}`)})
				if err != nil {
					t.Fatal(err)
				}
			}
			history, _, cost, err := repository.Resume(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			last := history[len(history)-1]
			if known && (last.Status != "ok" || last.Model != "actual-old" || cost != 12) {
				t.Fatalf("回复没找回：%+v 费用%d", last, cost)
			}
			if !known && (last.Status != "interrupted" || cost != 50) {
				t.Fatalf("未知费用丢失：%+v 费用%d", last, cost)
			}
			again, _, _, err := repository.Resume(ctx, id)
			if err != nil || len(again) != len(history) {
				t.Fatal("重复恢复新增了步骤")
			}
			// 当前租约外的执行者不得先扣款后联网，失败预留必须整体回滚。
			_, err = ledger.Reserve(llm.WithLease(t.Context(), "obsolete"), llm.Reservation{DiagnosisID: id, Model: "mock", Request: request, MaximumMicros: 1, DiagnosisLimitMicros: 100})
			if err == nil {
				t.Fatal("旧执行者仍能预留费用")
			}
			status, err := ledger.Status(t.Context())
			if err != nil || status.SpentMicros != cost {
				t.Fatal("拒绝预留后账本没有回滚")
			}
		})
	}
}

// TestIntegrationCurrency 旧人民币账本不能改成美元，也不能用美元账本替人民币诊断付款。
func TestIntegrationCurrency(t *testing.T) {
	repository, target := integrationStore(t)
	id, err := repository.Manual(t.Context(), target, "币种隔离")
	if err != nil {
		t.Fatal(err)
	}
	// 先建旧币种账户再尝试改名义单位，已有花费不能借配置修改变成另一种货币。
	cnyName, usdName := "cny-"+id, "usd-"+id
	defer func() {
		_, _ = repository.database.ExecContext(t.Context(), "DELETE FROM llm_attempts WHERE diagnosis_id=?", id)
		_, _ = repository.database.ExecContext(t.Context(), "DELETE FROM llm_budget WHERE account IN (?,?)", cnyName, usdName)
	}()
	if _, err = repository.NewCurrencyLedger(t.Context(), cnyName, 100, "CNY"); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.NewCurrencyLedger(t.Context(), cnyName, 100, "USD"); err == nil {
		t.Fatal("旧账本币种被更改")
	}
	usd, err := repository.NewCurrencyLedger(t.Context(), usdName, 100, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = usd.Reserve(t.Context(), llm.Reservation{DiagnosisID: id, Model: "mock", Request: json.RawMessage(`{}`), MaximumMicros: 10, DiagnosisLimitMicros: 100}); err == nil {
		t.Fatal("跨币种预留被放行")
	}
	status, err := usd.Status(t.Context())
	if err != nil || status.Currency != "USD" || status.SpentMicros != 0 {
		t.Fatal("拒绝后仍扣款或没有币种")
	}
}
