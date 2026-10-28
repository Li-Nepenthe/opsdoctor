// 本文件：提供默认免费运行的固定诊断脚本。
//
// Mock 也走真实工具、步骤保存和页面推送，只替换“模型决定下一步”这一环。
// 这样集成测试触发告警也不会产生费用；报告必须明确告诉用户这不是诊断结论。
//
// 主要内容：Client.Complete 先请求查看，再提交演示报告；observations 取得真实步骤号。
// 阅读建议：这里没有场景编号或针对某种故障的判断。

package mock

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// Model 是程序内固定的演示名称，页面和报告都据 mock 前缀显示横幅。
const Model = "mock-script-v1"

// Client 没有可变状态，多个诊断各自的脚本位置由消息历史决定。
type Client struct{}

// Complete 不访问网络；先走一轮查看，再基于保存过的步骤提交带标记的演示报告。
func (Client) Complete(ctx context.Context, request llm.Request) (llm.Response, error) {
	if ctx.Err() != nil {
		return llm.Response{}, ctx.Err()
	}
	evidence := observations(request.Messages)
	response := llm.Response{Model: Model, Message: llm.Message{Role: "assistant"}}
	if len(evidence) == 0 && len(request.Tools) > 1 {
		// 数据源缺失时没有 list_firing_alerts；脚本与真实模型一样只能用提供的工具。
		for _, tool := range request.Tools {
			if tool.Function.Name == "service_topology" || tool.Function.Name == "list_firing_alerts" {
				response.Message.ToolCalls = append(response.Message.ToolCalls, llm.ToolCall{ID: "mock-" + tool.Function.Name, Type: "function", Function: llm.Function{Name: tool.Function.Name, Arguments: "{}"}})
			}
		}
		if len(response.Message.ToolCalls) > 0 {
			return response, nil
		}
	}
	report := map[string]any{"summary": "工具往返流程已演示", "root_cause": map[string]any{"component": "unknown", "mechanism": "固定脚本不推断故障原因", "confidence": 0}, "evidence": evidence, "suggested_actions": []any{}, "unknowns": "需要真实诊断或工程师进一步判断"}
	raw, _ := json.Marshal(report)
	response.Message.ToolCalls = []llm.ToolCall{{ID: fmt.Sprintf("mock-report-%d", len(request.Messages)), Type: "function", Function: llm.Function{Name: "submit_report", Arguments: string(raw)}}}
	return response, nil
}

// observations 只引用工具实际返回的步骤号，不猜测并行查询完成的顺序。
func observations(messages []llm.Message) []any {
	items := []any{}
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		var payload struct {
			Step int `json:"step"`
		}
		if json.Unmarshal([]byte(message.Content), &payload) == nil && payload.Step > 0 {
			items = append(items, map[string]any{"step": payload.Step, "observation": "固定脚本已收到这一步的工具结果；没有自动判断根因"})
		}
	}
	return items
}
