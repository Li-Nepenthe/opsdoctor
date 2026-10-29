// 本文件：验证 Agent 暂停审批后仍能接回原来的工具调用。
//
// 人没有决定时不能继续问模型。恢复时也不能再申请一次，或者把审批结果接到
// 新的调用编号上。测试只用内存脚本，不访问业务系统和付费 API。
//
// 主要内容：TestApprovalReplay 检查等待、拒绝和恢复；TestMixedWrite 检查混合请求不写。
// 阅读建议：第一次 Run 留下断点，第二次 Run 使用新的 Agent 模拟进程重启。

package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// fakeApproval 只替代人工决定，调用编号和历史仍由真实 Agent 保存。
type fakeApproval struct {
	status string
	calls  int
	parent int
}

// List 只提供一个申请入口，不能从这份菜单批准自己。
func (*fakeApproval) List() []llm.Tool {
	return []llm.Tool{{Type: "function", Function: llm.ToolDefinition{Name: "warm_cache"}}}
}

// Handles 用固定名字识别写请求，不接受模型自称只读。
func (*fakeApproval) Handles(name string) bool { return name == "warm_cache" }

// Handle 检查恢复前后的同一个调用，再返回模拟的人为决定。
func (flow *fakeApproval) Handle(_ context.Context, _ string, parent int, call llm.ToolCall, _ map[int]bool) (bool, tools.Result, error) {
	flow.calls++
	if flow.parent == 0 {
		flow.parent = parent
	}
	if parent != flow.parent || call.ID != "write-1" {
		panic("恢复改变了审批关联")
	}
	return flow.status == "pending", tools.Result{Tool: "warm_cache", Status: flow.status, Summary: "人工决定", Data: []byte(`null`)}, nil
}

// TestApprovalReplay 批准、拒绝和过期都只产生一个原调用的结果，等待本身不消耗模型轮数。
func TestApprovalReplay(t *testing.T) {
	for _, status := range []string{"ok", "rejected", "expired"} {
		t.Run(status, func(t *testing.T) {
			// 第一次只允许申请；如果等待期间又调用模型，脚本立即失败。
			flow := &fakeApproval{status: "pending"}
			recorder := &memoryRecorder{}
			calls := 0
			model := modelFunc(func(_ context.Context, request llm.Request) (llm.Response, error) {
				calls++
				if calls == 1 {
					return answer(call("write-1", "warm_cache", `{"ids":[1]}`)), nil
				}
				if flow.status == "pending" {
					t.Fatal("等待期间继续调用模型")
				}
				last := request.Messages[len(request.Messages)-1]
				if last.ToolCallID != "write-1" || !strings.Contains(last.Content, flow.status) {
					t.Fatal("审批结果未接回原调用")
				}
				var value struct {
					Step int `json:"step"`
				}
				if json.Unmarshal([]byte(last.Content), &value) != nil {
					t.Fatal("结果格式损坏")
				}
				return answer(call("report", "submit_report", reportArgs(value.Step))), nil
			})
			loop, _ := New(model, "mock-first", llm.NewRedactor(), DefaultLimits())
			input := testInput(recorder, toolFunc(nil))
			input.Approvals = flow
			first, err := loop.Run(t.Context(), input)
			if err != nil || !first.WaitingApproval || calls != 1 || first.Rounds != 1 {
				t.Fatalf("等待结果=%+v %v", first, err)
			}
			input.History = append([]Step{}, recorder.steps...)
			flow.status = status
			resumed, _ := New(model, "mock-second", llm.NewRedactor(), DefaultLimits())
			result, err := resumed.Run(t.Context(), input)
			if err != nil || len(result.Report) == 0 || result.WaitingApproval || result.Rounds != 2 || result.ToolCalls != 1 || calls != 2 || flow.calls != 2 {
				t.Fatalf("恢复结果=%+v 调用=%d %v", result, calls, err)
			}
			count := 0
			for _, step := range recorder.steps {
				if step.Kind == "tool" && step.CallID == "write-1" {
					count++
				}
			}
			if count != 1 {
				t.Fatal("审批结果重复保存")
			}
		})
	}
}

// TestMixedWrite 写请求和报告同轮出现时，没有任何申请或执行机会。
func TestMixedWrite(t *testing.T) {
	// 即便模型把写操作说成报告的一部分，仍必须先经过独立的人工审批。
	flow := &fakeApproval{status: "pending"}
	recorder := &memoryRecorder{}
	calls := 0
	model := modelFunc(func(context.Context, llm.Request) (llm.Response, error) {
		calls++
		if calls == 1 {
			return answer(call("write-1", "warm_cache", `{"ids":[1]}`), call("bad-report", "submit_report", reportArgs(0))), nil
		}
		return answer(call("report", "submit_report", reportArgs(0))), nil
	})
	loop, _ := New(model, "mock-test", llm.NewRedactor(), DefaultLimits())
	input := testInput(recorder, toolFunc(nil))
	input.Approvals = flow
	result, err := loop.Run(t.Context(), input)
	if err != nil || len(result.Report) == 0 || flow.calls != 0 {
		t.Fatalf("混合请求越过审批边界：%+v %v", result, err)
	}
}
