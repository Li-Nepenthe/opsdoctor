// 本文件：确认模型只收到需要它判断的编号，不靠文字劝它别重复。
//
// 主要内容：TestPendingProtocol 验证材料、Schema 和 Mock 的往返；
// TestInvalidRules 验证坏编号在付费前被拒绝；TestRuleOverride 保留拒绝重复规则项的行为。
// 阅读建议：这些是通用的假项目，不使用任何场景答案来调评分。

package judge

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/eval/scoring"
)

// TestPendingProtocol 同时检查发出的列表和供应商看到的参数范围，避免只验其中一层。
func TestPendingProtocol(t *testing.T) {
	for _, test := range []struct {
		name        string
		ruled, want []int
	}{
		{"没有规则", nil, []int{1, 2, 3}},        // 所有项目都交给模型。
		{"中间由规则判定", []int{2}, []int{1, 3}},   // 保留原编号，不能把第三项变成第二项。
		{"全部由规则判定", []int{1, 2, 3}, []int{}}, // 仍评价机制，证据列表必须为空。
	} {
		t.Run(test.name, func(t *testing.T) {
			input := Input{Report: json.RawMessage(`{}`), Symptoms: json.RawMessage(`{}`), Milestones: []string{"甲", "乙", "丙"}}
			for _, number := range test.ruled {
				input.Rules = append(input.Rules, scoring.EvidenceResult{Milestone: number})
			}
			raw, tool, err := judgeRequest(input)
			if err != nil {
				t.Fatal(err)
			}
			var payload struct {
				Pending []pendingMilestone `json:"pending_milestones"`
				Old     json.RawMessage    `json:"milestones"`
			}
			if err = json.Unmarshal(raw, &payload); err != nil {
				t.Fatal(err)
			}
			got := []int{}
			for _, item := range payload.Pending {
				got = append(got, item.Number)
			}
			if !reflect.DeepEqual(got, test.want) || len(payload.Old) > 0 {
				t.Fatal("发送了错误的待评列表", string(raw))
			}
			var schema struct {
				Properties struct {
					Milestones struct {
						Min   int `json:"minItems"`
						Max   int `json:"maxItems"`
						Items struct {
							Properties struct {
								Number struct {
									Enum []int `json:"enum"`
								} `json:"milestone"`
							} `json:"properties"`
						} `json:"items"`
					} `json:"milestones"`
				} `json:"properties"`
			}
			// 同时核对编号集合与返回数量；少一项或补回规则项都不能默许。
			if err = json.Unmarshal(tool.Function.Parameters, &schema); err != nil {
				t.Fatal(err)
			}
			if len(test.want) > 0 && !reflect.DeepEqual(schema.Properties.Milestones.Items.Properties.Number.Enum, test.want) {
				t.Fatal("Schema 接受了规则项")
			}
			if schema.Properties.Milestones.Min != len(test.want) || schema.Properties.Milestones.Max != len(test.want) {
				t.Fatal("Schema 未限定返回数量")
			}

			if _, err = json.Marshal(llm.Request{Tools: []llm.Tool{tool}}); err != nil {
				t.Fatal("完整请求不能编码", err)
			}
			grade, _, err := Evaluate(t.Context(), Mock{}, "mock-judge", "audit", input)
			if err != nil || len(grade.Milestones) != len(test.want) {
				t.Fatal("Mock 没有使用相同协议", grade, err)
			}
		})
	}
}

// TestInvalidRules 内部规则损坏不能靠模型补救，更不能为此发付费请求。
func TestInvalidRules(t *testing.T) {
	for _, numbers := range [][]int{{0}, {2}, {1, 1}} { // 零编号、越界、重复各自拒绝。
		input := Input{Milestones: []string{"甲"}}
		for _, number := range numbers {
			input.Rules = append(input.Rules, scoring.EvidenceResult{Milestone: number})
		}
		if _, _, err := judgeRequest(input); err == nil {
			t.Fatal("接受了坏规则", numbers)
		}
	}
}

// TestRuleOverride 重现评委把规则项重复返回的失败，不能为让检查通过而忽略它。
func TestRuleOverride(t *testing.T) {
	input := Input{Milestones: []string{"甲", "乙"}, Steps: []Observation{{Step: 4}}, Rules: []scoring.EvidenceResult{{Milestone: 2}}}
	if _, err := Check([]byte(validGrade), input); err == nil {
		t.Fatal("模型重复返回规则项仍被采纳")
	}
}
