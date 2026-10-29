// 本文件：提供明确标记的免费评分脚本，供评测流程测试。
//
// 固定脚本不推理、不声称回答正确，只返回格式完整的“未判断”。这样的结果
// 可以检查数据库和页面接线，却不能混进真实模型成绩。
//
// 主要内容：Mock.Complete 读取项目列表，为每项返回有说明的占位评分。
// 阅读建议：Mock 没有 Key、网络客户端或任何工具执行能力。

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
	var input Input
	if len(request.Messages) != 2 || json.Unmarshal([]byte(request.Messages[1].Content), &input) != nil {
		return llm.Response{}, fmt.Errorf("Mock 评分输入不合法")
	}
	ruled := map[int]bool{}
	for _, rule := range input.Rules {
		ruled[rule.Milestone] = true
	}
	items := []any{}
	// 只补未由规则检查的项目，让测试也经过真实回复校验。
	for index := range input.Milestones {
		if !ruled[index+1] {
			items = append(items, map[string]any{"milestone": index + 1, "achieved": false, "steps": []int{}, "reason": "Mock 占位，未作真实判断"})
		}
	}
	raw, _ := json.Marshal(map[string]any{"mechanism": "incorrect", "reason": "Mock 模式，不是真实评分", "unrelated_action": false, "action_reason": "Mock 未判断", "milestones": items})
	return llm.Response{Model: "mock-judge-v1", Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{Type: "function", ID: "mock-grade", Function: llm.Function{Name: "submit_grade", Arguments: string(raw)}}}}}, nil
}
