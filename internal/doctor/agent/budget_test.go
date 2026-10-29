// 本文件：验证新预算不会把重复发送的历史过早算成调查结束。
//
// 这里用脚本精确指定每轮用量，不依赖供应商速度或真实费用。既检查允许继续的
// 路径，也检查越界后没有再执行工具，最后报告只能修正一次。
//
// 主要内容：TestBudgetRevision 检查累计用量；TestFinalCorrection 检查纠错次数；
// TestInputBound、TestHardBudget 检查上下文估计和发送前限制。
// 阅读建议：token 测试给出的是供应商用量，context 测试检查的是发送前的保守估计。

package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// TestBudgetRevision 第九轮仍能继续取证，不会再被旧的八万 token 触发线打断。
func TestBudgetRevision(t *testing.T) {
	calls := 0
	model := modelFunc(func(_ context.Context, request llm.Request) (llm.Response, error) {
		calls++
		response := answer(call("query", "service_topology", "{}"))
		response.Usage = llm.Usage{Input: 10000, CacheHit: 9000}
		if calls == 10 {
			response.Message.ToolCalls = []llm.ToolCall{call("report", "submit_report", reportArgs(0))}
		}
		return response, nil
	})
	loop, _ := New(model, "mock-test", llm.NewRedactor(), DefaultLimits())
	result, err := loop.Run(t.Context(), testInput(&memoryRecorder{}, tools.New(tools.Dependencies{})))
	if err != nil || result.Rounds != 10 || result.ReportMode != "voluntary" || result.Usage.Input != 100000 || result.Report == nil {
		t.Fatalf("提前结束：%+v %v", result, err)
	}
}

// TestFinalCorrection 第一次交报告故意编造步骤；只允许再修一次，不能反复占用额度。
func TestFinalCorrection(t *testing.T) {
	for _, corrected := range []bool{true, false} {
		// true 证明错误真的回到模型并可改正；false 证明第二次错误之后不会继续调用。
		t.Run(map[bool]string{true: "修正成功", false: "只给一次"}[corrected], func(t *testing.T) {
			calls := 0
			model := modelFunc(func(_ context.Context, request llm.Request) (llm.Response, error) {
				calls++
				if calls == 1 {
					return answer(call("query", "service_topology", "{}")), nil
				}
				if calls == 3 {
					if !strings.Contains(request.Messages[len(request.Messages)-1].Content, "不存在") {
						t.Fatal("校验错误没有回传")
					}
					if corrected {
						return answer(call("fixed", "submit_report", reportArgs(0))), nil
					}
				}
				return answer(call("invalid", "submit_report", reportArgs(999))), nil
			})
			limits := DefaultLimits()
			limits.Rounds = 1
			loop, _ := New(model, "mock-test", llm.NewRedactor(), limits)
			result, err := loop.Run(t.Context(), testInput(&memoryRecorder{}, tools.New(tools.Dependencies{})))
			if err != nil || calls != 3 || (result.Report != nil) != corrected || result.BudgetExceeded == corrected || result.ReportMode != "budget" {
				t.Fatalf("结果=%+v 次数=%d %v", result, calls, err)
			}
		})
	}
}

// TestInputBound 重发的历史按上次真实计量，只有新增部分按字节作最坏估计。
func TestInputBound(t *testing.T) {
	run := &session{lastInput: 10000, lastBytes: 50000}
	if got := run.inputBound(52000); got != 13024 {
		t.Fatalf("新增估计=%d", got)
	}
	if got := (&session{}).inputBound(2000); got != 3024 {
		t.Fatalf("首次估计=%d", got)
	}
}

// TestHardBudget 初始请求就超过上下文或累计量时，不允许为了最后报告再绕过上限。
func TestHardBudget(t *testing.T) {
	// 三个边界分别耗尽，检查拦截发生在模型发送之前，而非拿到超额账单之后。
	for _, boundary := range []string{"上下文", "累计量", "费用"} {
		t.Run(boundary, func(t *testing.T) {
			limits := DefaultLimits()
			if boundary == "上下文" {
				limits.ContextTokens = 1
			}
			if boundary == "累计量" {
				limits.Tokens = 1
			}
			calls := 0
			model := modelFunc(func(context.Context, llm.Request) (llm.Response, error) {
				calls++
				result := answer(call("q", "query", "{}"))
				result.CostMicros = limits.CostMicros
				return result, nil
			})
			loop, _ := New(model, "mock-test", llm.NewRedactor(), limits)
			toolbox := toolFunc(func(context.Context, string, json.RawMessage) tools.Result {
				return tools.Result{Tool: "query", Status: "ok", Data: json.RawMessage(`null`)}
			})
			result, err := loop.Run(t.Context(), testInput(&memoryRecorder{}, toolbox))
			want := 0
			if boundary == "费用" {
				want = 1
			}
			if err != nil || !result.BudgetExceeded || calls != want {
				t.Fatalf("越界：calls=%d result=%+v err=%v", calls, result, err)
			}
		})
	}
}
