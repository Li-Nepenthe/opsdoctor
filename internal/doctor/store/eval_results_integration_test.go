// 本文件：用专用医生测试库验证结果槽位的续跑边界。
//
// 主要内容：TestIntegrationEvalResults 检查冻结输入、幂等关联、终态要求和失败不可覆盖。
// 阅读建议：模拟崩溃后重试相同编号，不访问靶场、不调用付费模型。

package store

import (
	"encoding/json"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// TestIntegrationEvalResults 按“先保存槽位、后建任务、最后保存结果”的真实顺序逐处重试。
func TestIntegrationEvalResults(t *testing.T) {
	repository, input := evaluationFixture(t)
	ctx := llm.WithLease(t.Context(), "eval-results-owner")
	if err := repository.SetEvaluationMode(ctx, input.Target, input.RunID, true); err != nil {
		t.Fatal(err)
	}
	slot := EvalResult{Key: input.Key, RunID: input.RunID, Scenario: "S01", Iteration: 1, Group: "E1", Position: 1, FaultStartedAt: input.FaultStartedAt, Input: input.Symptoms}
	saved, err := repository.PrepareEvalResult(ctx, slot)
	if err != nil {
		t.Fatal(err)
	}
	// 新通知到了，但续跑必须沿用第一份输入，不能把环境变化混入旧任务。
	slot.Input = json.RawMessage(`{"alerts":["later"]}`)
	repeated, err := repository.PrepareEvalResult(ctx, slot)
	if err != nil || !sameJSON(saved.Input, repeated.Input) {
		t.Fatalf("输入被覆盖：%v", err)
	}
	if err = repository.FinishEvalResult(ctx, slot.Key, json.RawMessage(`{"status":"failed"}`)); err == nil {
		t.Fatal("未建任务就写了终态")
	}
	id, err := repository.Evaluation(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = repository.AttachEvalDiagnosis(ctx, slot.Key, id); err != nil {
			t.Fatal(err)
		}
	}
	if err = repository.FinishEvalResult(ctx, slot.Key, json.RawMessage(`{"status":"failed"}`)); err == nil {
		t.Fatal("pending 任务就写了终态")
	}
	if ok, err := repository.StartTask(ctx, id, input.ExpectedModel, "test", "mock"); err != nil || !ok {
		t.Fatal(err)
	}
	if err = repository.EndTask(ctx, id, "failed", agent.Result{}, "测试中的失败"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = repository.FinishEvalResult(ctx, slot.Key, json.RawMessage(`{"status":"failed"}`)); err != nil {
			t.Fatal(err)
		}
	}
	if err = repository.FinishEvalResult(ctx, slot.Key, json.RawMessage(`{"status":"succeeded"}`)); err == nil {
		t.Fatal("失败被成功覆盖")
	}
	rows, err := repository.EvalResults(ctx, input.RunID)
	if err != nil || len(rows) != 1 || rows[0].DiagnosisID != id || !sameJSON(rows[0].Data, json.RawMessage(`{"status":"failed"}`)) {
		t.Fatalf("槽位重复或遗漏：%+v %v", rows, err)
	}
}
