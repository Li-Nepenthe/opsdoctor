// 本文件：证明无工具组引用的是实际告警，故障时间和证据编号会随请求一起保存。
//
// 主要内容：TestInputEvidence 检查正常引用与编造引用；TestInputReplay 检查重启不会把 E0 当成预算收尾。
// 阅读建议：全程只用脚本模型，测试既不读取 Key，也不发送外部请求。

package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// TestInputEvidence 先故意引用模型步骤，再改成输入步骤，保证校验不是“任意已有编号都行”。
func TestInputEvidence(t *testing.T) {
	recorder := &memoryRecorder{}
	input := testInput(recorder, tools.New(tools.Dependencies{}).ForDiagnosis(time.Time{}, map[string]bool{}))
	input.InputOnly, input.FaultStartedAt = true, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	model := modelFunc(func(_ context.Context, request llm.Request) (llm.Response, error) {
		calls++
		if len(request.Tools) != 1 || finalRequest(request) || !strings.Contains(request.Messages[0].Content, "2026-10-01T00:00:00Z") {
			t.Fatal("无工具组的菜单、时间或收尾状态不正确")
		}
		if strings.Contains(request.Messages[2].Content, "demo-secret-value") || !strings.Contains(request.Messages[2].Content, `"input_evidence_step":1`) {
			t.Fatal("告警没有脱敏，或没有真实步骤号")
		}
		if calls == 1 {
			return answer(call("bad", "submit_report", reportArgs(3))), nil // 本轮 llm 的编号是 3，但它不是告警。
		}
		if !strings.Contains(request.Messages[len(request.Messages)-1].Content, "invalid_argument") {
			t.Fatal("编造的证据来源没有退回")
		}
		return answer(call("fixed", "submit_report", reportArgs(1))), nil
	})
	loop, err := New(model, "mock-input", llm.NewRedactor(), DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	result, err := loop.Run(t.Context(), input)
	if err != nil || result.Report == nil || result.ReportMode != "voluntary" || calls != 2 {
		t.Fatalf("输入证据流程：%+v %v，调用 %d 次", result, err, calls)
	}
}

// TestInputReplay 用一次中断的 E0 请求重建对话，仍然保留普通轮次和原来的输入编号。
func TestInputReplay(t *testing.T) {
	// 第一条是已保存输入，第二条是没有取得回复的模型请求，模拟恰好在网络调用中停机。
	inputStep := Step{Seq: 1, Kind: "system", EventKey: "diagnosis-input", Status: "input_evidence", Result: "告警输入"}
	prior := llm.Request{Messages: []llm.Message{{Role: "system", Content: "原提示"}, {Role: "user", Content: `{"input_evidence_step":1}`}}, Tools: []llm.Tool{reportDefinition([]string{"unknown"})}}
	raw, _ := json.Marshal(prior)
	recorder := &memoryRecorder{steps: []Step{inputStep, {Seq: 2, Kind: "llm", Status: "failed", Args: raw, Result: `{}`}}}
	input := testInput(recorder, tools.New(tools.Dependencies{}).ForDiagnosis(time.Time{}, map[string]bool{}))
	input.InputOnly, input.History = true, append([]Step{}, recorder.steps...)
	model := modelFunc(func(_ context.Context, request llm.Request) (llm.Response, error) {
		if finalRequest(request) {
			t.Fatal("恢复 E0 被误认成预算收尾")
		}
		return answer(call("report", "submit_report", reportArgs(1))), nil
	})
	loop, _ := New(model, "mock-input", llm.NewRedactor(), DefaultLimits())
	result, err := loop.Run(t.Context(), input)
	if err != nil || result.ReportMode != "voluntary" || result.Report == nil {
		t.Fatalf("恢复 E0：%+v %v", result, err)
	}
	count := 0
	for _, step := range recorder.steps {
		if step.EventKey == "diagnosis-input" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("初始输入重复保存 %d 次", count)
	}
}
