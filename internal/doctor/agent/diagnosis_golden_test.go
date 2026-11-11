// 本文件：用同一次诊断，对照发给模型的请求和记入仓储的步骤。
//
// 每轮只请求一个工具，使审计顺序稳定。虚拟时钟也固定了工具结果里的耗时，
// 因此无需删减时间、费用或证据字段就能逐字节比较。

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// TestDiagnosisTranscriptGolden 从初始告警开始，依次保存查证、参数错误和预算最后一轮的报告。
func TestDiagnosisTranscriptGolden(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		recorder := &memoryRecorder{}
		registry := tools.New(tools.Dependencies{Profile: "订单服务通过库存服务检查库存。"})
		input := testInput(recorder, registry)
		input.FaultStartedAt = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
		limits := DefaultLimits()
		limits.Rounds = 2

		// 在 Complete 入口立即编码，后续追加消息不能改变已经采集的请求。
		var requests bytes.Buffer
		script := []llm.ToolCall{
			call("read", "service_topology", "{}"),
			call("bad-args", "service_topology", `{"extra":"参数错误"}`),
			call("report", "submit_report", reportArgs(4)), // 第四步是本次诊断实际保存的拓扑证据。
		}
		round := 0
		model := modelFunc(func(_ context.Context, request llm.Request) (llm.Response, error) {
			if round >= len(script) {
				t.Fatalf("提交报告后又请求第 %d 轮", round+1)
			}
			if err := json.NewEncoder(&requests).Encode(request); err != nil {
				t.Fatal(err)
			}
			response := answer(script[round])
			round++
			return response, nil
		})
		doctor, err := New(model, "mock-test", llm.NewRedactor(), limits)
		if err != nil {
			t.Fatal(err)
		}
		result, err := doctor.Run(t.Context(), input)
		if err != nil || result.Report == nil || result.ReportMode != "budget" || round != len(script) {
			t.Fatalf("诊断未按脚本交付报告：模式 %q，轮数 %d，错误 %v", result.ReportMode, round, err)
		}

		var steps bytes.Buffer
		for _, step := range recorder.steps {
			if err := json.NewEncoder(&steps).Encode(step); err != nil {
				t.Fatal(err)
			}
		}
		checkTranscript(t, "testdata/diagnosis_requests.golden.jsonl", requests.Bytes())
		checkTranscript(t, "testdata/diagnosis_steps.golden.jsonl", steps.Bytes())
	})
}

// checkTranscript 每行保存一个完整的 JSON 对象，报错指出变化始于哪次请求或审计步骤。
func checkTranscript(t *testing.T, path string, got []byte) {
	t.Helper()
	if bytes.Contains(got, []byte("demo-secret-value")) {
		t.Fatalf("%s 含未脱敏的症状", path)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		offset := 0
		for offset < min(len(got), len(want)) && got[offset] == want[offset] {
			offset++
		}
		t.Fatalf("%s 第 %d 行不同（字节偏移 %d）", path, bytes.Count(got[:offset], []byte("\n"))+1, offset)
	}
}
