// 本文件：把需要模型填写的项目和程序已经判定的项目分开。
//
// 只在提示里说“不要重复”仍可能得到完整列表。这里给每个待评项目明确的编号，
// 工具 Schema 也只接受这些编号；返回后仍由 Check 核对，不能只信模型遵守了说明。
//
// 主要内容：pendingMilestones 计算待评集合；judgeRequest 同时生成材料与工具定义。
// 阅读建议：编号来自当前输入，不包含任何场景特例；评分尺度仍在 rubric.md。

package judge

import (
	"encoding/json"
	"fmt"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// pendingMilestone 保留完整列表中的原编号，删除规则项后不能重新从一开始编号。
type pendingMilestone struct {
	Number      int    `json:"milestone"`   // Check 按原编号合并规则与模型结果。
	Description string `json:"description"` // 当前需要判断的证据要求。
}

// pendingMilestones 在付费前检查规则编号，内部材料损坏时不能交给模型猜。
func pendingMilestones(input Input) ([]pendingMilestone, error) {
	ruled := map[int]bool{}
	for _, item := range input.Rules {
		if item.Milestone < 1 || item.Milestone > len(input.Milestones) || ruled[item.Milestone] {
			return nil, fmt.Errorf("规则项目编号无效或重复")
		}
		ruled[item.Milestone] = true
	}
	values := []pendingMilestone{}
	for index, description := range input.Milestones {
		if !ruled[index+1] {
			values = append(values, pendingMilestone{Number: index + 1, Description: description})
		}
	}
	return values, nil
}

// judgeRequest 只发送明确的待评列表，规则结果作为只读背景保留。
func judgeRequest(input Input) (json.RawMessage, llm.Tool, error) {
	pending, err := pendingMilestones(input)
	if err != nil {
		return nil, llm.Tool{}, err
	}
	raw, err := json.Marshal(map[string]any{"reference": input.Truth, "report": input.Report, "symptoms": input.Symptoms, "steps": input.Steps, "pending_milestones": pending, "rule_results": input.Rules})
	if err != nil {
		return nil, llm.Tool{}, err
	}
	numbers := []int{}
	for _, item := range pending {
		numbers = append(numbers, item.Number)
	}
	number := map[string]any{"type": "integer", "minimum": 1}
	if len(numbers) > 0 {
		number["enum"] = numbers // 不只限制个数，还限制可填写的原编号。
	}
	// 空待评集合使用 maxItems=0；不能产生不合法的空 enum。
	text := map[string]any{"type": "string"}
	properties := map[string]any{
		"milestone": number,
		"achieved":  map[string]any{"type": "boolean"},
		"steps":     map[string]any{"type": "array", "items": map[string]any{"type": "integer", "minimum": 1}},
		"reason":    text,
	}
	schema, err := json.Marshal(map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"mechanism", "reason", "milestones", "unrelated_action", "action_reason"},
		"properties": map[string]any{
			"mechanism": map[string]any{"type": "string", "enum": []string{"correct", "partial", "incorrect"}},
			"reason":    text, "action_reason": text, "unrelated_action": map[string]any{"type": "boolean"},
			"milestones": map[string]any{"type": "array", "minItems": len(numbers), "maxItems": len(numbers), "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"milestone", "achieved", "steps", "reason"}, "properties": properties}},
		},
	})
	if err != nil {
		return nil, llm.Tool{}, err
	}
	return raw, llm.Tool{Type: "function", Function: llm.ToolDefinition{Name: "submit_grade", Description: "按评分标准提交机制和待评项目；milestones 恰好填写 pending_milestones 列出的编号，不重复 rule_results。", Parameters: schema}}, nil
}
