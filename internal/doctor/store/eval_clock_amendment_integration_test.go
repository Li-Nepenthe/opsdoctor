// 本文件：用专用医生库证明校时补跑只适用于明确批准的原取消任务。
//
// 主要内容：TestIntegrationEvalClockAmendment 检查批准前拒绝、批准后幂等、原结果保留。
// 阅读建议：虚构请求审计含一次成功，证明这是明确的校时例外，而非放宽普通网络规则。

package store

import (
	"encoding/json"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// TestIntegrationEvalClockAmendment 不访问模型，只在测试库模拟时钟取消和人工批准。
func TestIntegrationEvalClockAmendment(t *testing.T) {
	repository, input := evaluationFixture(t)
	ctx := llm.WithLease(t.Context(), "clock-amend-owner")
	if err := repository.SetEvaluationMode(ctx, input.Target, input.RunID, true); err != nil {
		t.Fatal(err)
	}
	slot := EvalResult{Key: input.Key, RunID: input.RunID, Scenario: "S02", Iteration: 2, Group: "E1", Position: 1, FaultStartedAt: input.FaultStartedAt, Input: input.Symptoms}
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
	if ok, err := repository.StartTask(ctx, id, input.ExpectedModel, "test", "mock"); err != nil || !ok {
		t.Fatal(err)
	}
	if err = repository.EndTask(ctx, id, "cancelled", agent.Result{}, "取消"); err != nil {
		t.Fatal(err)
	}
	if err = repository.FinishEvalResult(ctx, slot.Key, json.RawMessage(`{"status":"cancelled"}`)); err != nil {
		t.Fatal(err)
	}
	before, after := json.RawMessage(`{"version":"old"}`), json.RawMessage(`{"version":"new"}`)
	reason := "用户明确批准校时门槛与这条中断的一次补跑"
	if err = repository.AmendEvalClock(ctx, input.RunID, slot.Key, reason, before, after); err == nil {
		t.Fatal("没有时钟审计也允许批准")
	}
	if _, err = repository.PrepareEvalMakeup(ctx, slot.Key); err == nil {
		t.Fatal("普通取消被自动补跑")
	}
	trial := EvalTrial{RunID: input.RunID, Scenario: "S02", Iteration: 2, State: "diagnosing"}
	if err = repository.InterruptEval(ctx, trial, "容器时钟超出边界"); err != nil {
		t.Fatal(err)
	}
	// 人工构造的成功请求只说明模型已经开始回答，绝不发生真实费用。
	if _, err = repository.NewCurrencyLedger(ctx, input.Target, 1000, "CNY"); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.database.ExecContext(ctx, "INSERT INTO llm_attempts(id,diagnosis_id,account,requested_model,request,maximum_micros,charged_micros,status,settled_at) VALUES(?,?,?,?,'{}',100,25,'ok',UTC_TIMESTAMP(6))", newID(), id, input.Target, input.ExpectedModel); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = repository.AmendEvalClock(ctx, input.RunID, slot.Key, reason, before, after); err != nil {
			t.Fatal(err)
		}
		makeup, err := repository.PrepareEvalMakeup(ctx, slot.Key)
		if err != nil || makeup.RetryOf != slot.Key || makeup.Attempt != 1 {
			t.Fatal(makeup, err)
		}
		if _, err = repository.PrepareEvalMakeup(ctx, makeup.Key); err == nil {
			t.Fatal("允许无限补跑")
		}
	}
	if err = repository.AmendEvalClock(ctx, input.RunID, slot.Key, reason, before, json.RawMessage(`{"version":"third"}`)); err == nil {
		t.Fatal("批准被覆盖")
	}
	versions, err := repository.EvalVersions(ctx, input.RunID, before)
	if err != nil || !sameJSON(versions, after) {
		t.Fatal(string(versions), err)
	}
	if _, err = repository.EvalVersions(ctx, input.RunID, json.RawMessage(`{}`)); err == nil {
		t.Fatal("错误原版本被接受")
	}
	rows, err := repository.EvalResults(ctx, input.RunID)
	if err != nil || len(rows) != 2 || len(rows[0].Data) == 0 {
		t.Fatal("原取消或新槽位丢失", err)
	}
}
