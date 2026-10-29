// 本文件：用专用测试库证明评委走独立账户，续跑不重复创建审计记录。
//
// 主要内容：TestIntegrationEvalJudge 检查账户、调度隔离和不可覆盖结果；
// TestIntegrationEvalLock 验证两个命令不能同时接手同一个批次。
// 阅读建议：费用都是测试生成的整数，不会调用任何模型接口。

package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// TestIntegrationEvalJudge 用一份已失败的医生结果创建评分审计，模拟响应丢失后的恢复。
func TestIntegrationEvalJudge(t *testing.T) {
	repository, input := evaluationFixture(t)
	ctx := t.Context()
	if err := repository.SetEvaluationMode(ctx, input.Target, input.RunID, true); err != nil {
		t.Fatal(err)
	}
	slot := EvalResult{Key: input.Key, RunID: input.RunID, Scenario: "S01", Iteration: 1, Group: "E1", Position: 1, FaultStartedAt: input.FaultStartedAt, Input: input.Symptoms}
	if _, err := repository.PrepareEvalResult(ctx, slot); err != nil {
		t.Fatal(err)
	}
	id, err := repository.Evaluation(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if err = repository.AttachEvalDiagnosis(ctx, slot.Key, id); err != nil {
		t.Fatal(err)
	}
	owned := llm.WithLease(ctx, "judge-test-doctor")
	if ok, err := repository.StartTask(owned, id, input.ExpectedModel, "test", "mock"); err != nil || !ok {
		t.Fatal(err)
	}
	if err = repository.EndTask(owned, id, "failed", agent.Result{}, "测试失败"); err != nil {
		t.Fatal(err)
	}
	if err = repository.FinishEvalResult(ctx, slot.Key, json.RawMessage(`{"status":"failed"}`)); err != nil {
		t.Fatal(err)
	}
	account := input.Target + "-judge"
	ledger, err := repository.NewCurrencyLedger(ctx, account, 100, "CNY")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = repository.database.ExecContext(context.Background(), "DELETE FROM llm_budget WHERE account IN (?,?)", account, account+"-wrong")
	})
	value, err := repository.PrepareEvalJudge(ctx, slot.Key, "mock-judge", "mock", account, "test-v1", json.RawMessage(`{"reference":"fixture"}`))
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := repository.PrepareEvalJudge(ctx, slot.Key, "mock-judge", "mock", account, "test-v1", json.RawMessage(`{"reference":"fixture"}`))
	if err != nil || repeated.ID != value.ID {
		t.Fatal("评分审计重复", err)
	}
	if _, err = repository.PrepareEvalJudge(ctx, slot.Key, "mock-other", "mock", account, "test-v1", value.Input); err == nil {
		t.Fatal("评分悄悄换模型")
	}
	if tasks, err := repository.Pending(ctx); err != nil {
		t.Fatal(err)
	} else {
		for _, task := range tasks {
			if task.ID == value.ID {
				t.Fatal("医生领取了评委任务")
			}
		}
	}
	if ok, err := repository.StartEvalJudge(ctx, value.ID); err != nil || !ok {
		t.Fatal(err)
	}
	if err = repository.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if detail, err := repository.Detail(ctx, value.ID); err != nil || detail.Status != "running" {
		t.Fatal("医生恢复器错误结束评委", err)
	}
	wrong, err := repository.NewCurrencyLedger(ctx, account+"-wrong", 100, "USD")
	if err != nil {
		t.Fatal(err)
	}
	reservation := llm.Reservation{DiagnosisID: value.ID, CallID: "eval-grade-v1", Model: "mock-judge", MaximumMicros: 20, MaximumTokens: 10, DiagnosisLimitMicros: 100, Request: json.RawMessage(`{}`)}
	if _, err = wrong.Reserve(ctx, reservation); err == nil {
		t.Fatal("评委绕过自己的人民币账户")
	}
	if _, err = ledger.Reserve(ctx, reservation); err != nil {
		t.Fatal(err)
	}
	// 模拟预留后进程退出。恢复只保留未知费用，不能凭空退钱或再次发送。
	response, err := repository.SavedEvalJudgeResponse(ctx, value.ID)
	if err != nil || response.CostMicros != 20 || len(response.Message.ToolCalls) != 0 {
		t.Fatalf("恢复=%+v %v", response, err)
	}
	for range 2 {
		if err = repository.FinishEvalJudge(ctx, slot.Key, value.ID, json.RawMessage(`{"status":"failed","reason":"中断"}`), response, nil, "中断"); err != nil {
			t.Fatal(err)
		}
	}
	if err = repository.FinishEvalJudge(ctx, slot.Key, value.ID, json.RawMessage(`{"status":"ok"}`), response, nil, ""); err == nil {
		t.Fatal("评分失败被覆盖")
	}
	status, err := ledger.Status(ctx)
	if err != nil || status.SpentMicros != 20 {
		t.Fatal("未知费用丢失", err)
	}
}

// TestIntegrationEvalLock 第一执行者离开前第二个必须失败，离开后可以续跑。
func TestIntegrationEvalLock(t *testing.T) {
	repository, target := integrationStore(t)
	if err := repository.WithEvalLock(t.Context(), target, func(ctx context.Context) error {
		if err := repository.WithEvalLock(ctx, target, func(context.Context) error { t.Error("第二个执行者进入"); return nil }); err == nil {
			t.Error("重复启动未拒绝")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.WithEvalLock(t.Context(), target, func(context.Context) error { return nil }); err != nil {
		t.Fatal("退出后未释放锁", err)
	}
}
