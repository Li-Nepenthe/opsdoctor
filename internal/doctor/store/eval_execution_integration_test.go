// 本文件：用专用医生测试库验证每份评测诊断的模型与账户绑定。
//
// 主要内容：TestIntegrationEvalExecution 检查重试不能改绑定、租约不能换币种、账本不能串账户。
// TestIntegrationPairedSlots 检查同一次故障的两个 E1 各占一个独立槽位。
// 阅读建议：两个模型都是 Mock；虚构账户只用于检查拒绝路径，不发送付费请求。

package store

import (
	"encoding/json"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// TestIntegrationEvalExecution 在真实事务边界验证绑定，不只检查配置解析。
func TestIntegrationEvalExecution(t *testing.T) {
	repository, input := evaluationFixture(t)
	ctx := llm.WithLease(t.Context(), "paired-owner")
	input.ExecutionProfile, input.Provider, input.Account, input.Currency = "second", "mock", input.Target, "USD"
	if err := repository.SetEvaluationMode(ctx, input.Target, input.RunID, true); err != nil {
		t.Fatal(err)
	}
	id, err := repository.Evaluation(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := repository.Pending(ctx)
	if err != nil || len(queued) != 1 || queued[0].ExecutionProfile != input.ExecutionProfile {
		t.Fatalf("排队丢失执行配置：%+v %v", queued, err)
	}
	// 同一编号不能在重试时改账户，否则未知预留额可能被留在旧账户后绕过。
	changed := input
	changed.Account += "-other"
	if _, err := repository.Evaluation(ctx, changed); err == nil {
		t.Fatal("同一评测编号更换了账户")
	}
	if ok, err := repository.StartTask(ctx, id, input.ExpectedModel, "test", "mock"); err == nil || ok {
		t.Fatal("默认仓储领走了另一个币种的评测")
	}
	bound := repository.ForExecution(input.ExecutionProfile, input.Account, input.Currency)
	if ok, err := bound.StartTask(ctx, id, input.ExpectedModel, "test", "mock"); err != nil || !ok {
		t.Fatalf("绑定正确仍被拒绝：%v", err)
	}
	wrong, err := repository.NewCurrencyLedger(ctx, input.Account+"-other", 1000, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wrong.Reserve(ctx, llm.Reservation{DiagnosisID: id, Model: input.ExpectedModel, MaximumMicros: 100, Request: []byte(`{}`)}); err == nil {
		t.Fatal("直接调用账本绕过了账户绑定")
	}
	tasks, err := repository.Pending(ctx)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("正在执行的任务又进入候选：%+v %v", tasks, err)
	}
}

// TestIntegrationPairedSlots 保持场景轮次、组别和故障时间相同，只改变模型执行配置。
func TestIntegrationPairedSlots(t *testing.T) {
	repository, input := evaluationFixture(t)
	for index, profile := range []string{"primary", "comparison"} {
		value := EvalResult{Key: input.Key + "/" + profile, ExecutionProfile: profile, RunID: input.RunID, Scenario: "S01", Iteration: 1, Group: "E1", Position: index + 1, FaultStartedAt: input.FaultStartedAt, Input: json.RawMessage(`{"alerts":[]}`)}
		// 每个配置重试两次，仍只应得到两行，而不是四行或挤在同一行。
		for range 2 {
			if saved, err := repository.PrepareEvalResult(t.Context(), value); err != nil || saved.ExecutionProfile != profile {
				t.Fatalf("同次故障配对槽位冲突：%+v %v", saved, err)
			}
		}
	}
	rows, err := repository.EvalResults(t.Context(), input.RunID)
	if err != nil || len(rows) != 2 || rows[0].FaultStartedAt != rows[1].FaultStartedAt {
		t.Fatalf("配对槽位丢失或不是同次故障：%+v %v", rows, err)
	}
}
