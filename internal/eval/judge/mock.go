// 本文件：提供明确标记为 Mock 的固定评分结果，供离线测试走通评委协议。脚本只处理本轮材料，没有模型凭据或工具执行能力。

package judge

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// Mock 只实现评委协议，不替代医生的诊断脚本。
type Mock struct{}

// Complete 所有结论都写明固定脚本，不能用参考答案给自己制造正确结果。
func (Mock) Complete(ctx context.Context, request llm.Request) (llm.Response, error) {
	if ctx.Err() != nil {
		return llm.Response{}, ctx.Err()
	}
	var input struct {
		Pending []pendingMilestone `json:"pending_milestones"`
	}
	if len(request.Messages) != 2 || json.Unmarshal([]byte(request.Messages[1].Content), &input) != nil || input.Pending == nil {
		return llm.Response{}, fmt.Errorf("Mock 评分输入不合法")
	}
	items := []any{}
	// 只补未由规则检查的项目，让测试也经过真实回复校验。
	for _, item := range input.Pending {
		items = append(items, map[string]any{"milestone": item.Number, "achieved": false, "steps": []int{}, "reason": "Mock 占位，未作真实判断"})
	}
	raw, _ := json.Marshal(map[string]any{"mechanism": "incorrect", "reason": "Mock 模式，不是真实评分", "unrelated_action": false, "action_reason": "Mock 未判断", "milestones": items})
	return llm.Response{Model: "mock-judge-v1", Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{Type: "function", ID: "mock-grade", Function: llm.Function{Name: "submit_grade", Arguments: string(raw)}}}}}, nil
}
