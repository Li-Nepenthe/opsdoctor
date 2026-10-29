// 本文件：用专用医生测试库验证结果槽位的续跑边界。
//
// 主要内容：TestIntegrationEvalResults 检查冻结输入、幂等关联、终态要求和失败不可覆盖。
// TestIntegrationEvalMakeup 检查一次补跑的上限和原始记录保留。
// 阅读建议：模拟崩溃后重试相同编号，不访问靶场、不调用付费模型。

package store

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// TestIntegrationEvalResults 按“先保存槽位、后建任务、最后保存结果”的真实顺序逐处重试。
func TestIntegrationEvalResults(t *testing.T) {
	repository, input := evaluationFixture(t)
	// 纳秒尾数会让 MySQL 默认向上取整；槽位和任务都必须主动截断到微秒。
	input.FaultStartedAt = time.Date(2026, 10, 29, 12, 20, 2, 15277600, time.UTC)
	ctx := llm.WithLease(t.Context(), "eval-results-owner")
	if err := repository.SetEvaluationMode(ctx, input.Target, input.RunID, true); err != nil {
		t.Fatal(err)
	}
	slot := EvalResult{Key: input.Key, RunID: input.RunID, Scenario: "S01", Iteration: 1, Group: "E1", Position: 1, FaultStartedAt: input.FaultStartedAt, Input: input.Symptoms}
	saved, err := repository.PrepareEvalResult(ctx, slot)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.FaultStartedAt.Equal(input.FaultStartedAt.Truncate(time.Microsecond)) {
		t.Fatal("故障开始时间被四舍五入，续跑会误判为另一轮")
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

// TestIntegrationEvalMakeup 用失败审计模拟断网；重复申请也只有一个新槽位，不发送 HTTP。
func TestIntegrationEvalMakeup(t *testing.T) {
	repository, input := evaluationFixture(t)
	ctx := llm.WithLease(t.Context(), "eval-makeup-owner")
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
	if ok, err := repository.StartTask(ctx, id, input.ExpectedModel, "test", "mock"); err != nil || !ok {
		t.Fatal(err)
	}
	// 即使错误文字说“断网”，没有请求审计时也不允许补跑。
	if err = repository.EndTask(ctx, id, "failed", agent.Result{}, "网络失败"); err != nil {
		t.Fatal(err)
	}
	if err = repository.FinishEvalResult(ctx, slot.Key, json.RawMessage(`{"status":"failed"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.PrepareEvalMakeup(ctx, slot.Key); err == nil {
		t.Fatal("只有错误文字也允许了补跑")
	}
	// 写入虚构请求的审计状态，不真的预留或花费，也不更改任何真实账户。
	account := input.Target
	if _, err = repository.NewCurrencyLedger(ctx, account, 1000, "CNY"); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.database.ExecContext(ctx, "INSERT INTO llm_attempts(id,diagnosis_id,account,requested_model,request,maximum_micros,charged_micros,status,settled_at) VALUES(?,?,?,?,'{}',100,100,'network_error',UTC_TIMESTAMP(6))", newID(), id, account, input.ExpectedModel); err != nil {
		t.Fatal(err)
	}
	var makeup EvalResult
	for range 3 {
		makeup, err = repository.PrepareEvalMakeup(ctx, slot.Key)
		if err != nil || makeup.RetryOf != slot.Key || makeup.Attempt != 1 || !sameJSON(makeup.Input, input.Symptoms) || !makeup.FaultStartedAt.Equal(input.FaultStartedAt) {
			t.Fatalf("补跑不一致：%+v %v", makeup, err)
		}
	}
	if _, err = repository.PrepareEvalMakeup(ctx, makeup.Key); err == nil {
		t.Fatal("允许为补跑再补跑")
	}
	if _, err = repository.PrepareEvalResult(ctx, makeup); err == nil {
		t.Fatal("普通入口允许直接创建补跑")
	}
	rows, err := repository.EvalResults(ctx, input.RunID)
	if err != nil || len(rows) != 2 || rows[0].DiagnosisID != id || len(rows[0].Data) == 0 || rows[1].Attempt != 1 {
		t.Fatalf("原失败丢失或补跑重复：%+v %v", rows, err)
	}
}
