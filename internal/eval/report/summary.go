// 本文件：从医生保存的真实任务和步骤提取试跑结果。
//
// 汇总不重新调用模型。它保留报告、失败原因和每种实际模型名；组件精确比对只是
// 一个维度，不能把它直接说成机制已经正确。
//
// 主要内容：Summary 保存可导出字段；Summarize 按审计步骤检查模型是否中途改变。
// 阅读建议：没有报告时 Component 为空，不能用标准答案补齐。

package report

import (
	"encoding/json"
	"sort"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
)

// Summary 同时保留定量数据和原报告，之后评委只能对实际存在的结果评分。
type Summary struct {
	DiagnosisID    string          `json:"diagnosis_id"`    // 可回到完整步骤审计。
	Status         string          `json:"status"`          // 成功、失败和预算终态都保留。
	Model          string          `json:"model"`           // 诊断开始时选用的模型。
	ActualModels   []string        `json:"actual_models"`   // 每一步实际返回的模型名，去重后排序。
	MixedModel     bool            `json:"mixed_model"`     // true 时不进入单模型统计。
	Component      string          `json:"component"`       // 实际报告写出的组件。
	ComponentMatch bool            `json:"component_match"` // 与标准答案精确比较，机制另评。
	Rounds         int             `json:"rounds"`          // 包含最终报告机会。
	Tools          int             `json:"tools"`           // 实际工具调用数。
	InputTokens    int64           `json:"input_tokens"`    // 医生步骤中的累计输入，包括缓存。
	OutputTokens   int64           `json:"output_tokens"`   // 累计输出。
	CostMicros     int64           `json:"cost_micros"`     // 费用账本金额，含未知请求预留。
	Currency       string          `json:"currency"`        // CNY 和 USD 不相加。
	Seconds        float64         `json:"seconds"`         // 从医生开始执行到终态的墙钟时间。
	ReportMode     string          `json:"report_mode"`     // 主动提交还是预算要求收尾。
	BudgetReason   string          `json:"budget_reason"`   // 预算终态的具体限制。
	Error          string          `json:"error"`           // 已脱敏的失败说明。
	Report         json.RawMessage `json:"report"`          // 原始报告，不修正文案或根因。
}

// Summarize 将步骤与开始时的模型逐一比较；实际模型变化的诊断不能归到单一模型名下。
func Summarize(detail store.Detail, expected string) Summary {
	value := Summary{DiagnosisID: detail.ID, Status: detail.Status, Model: detail.Model, Rounds: detail.Rounds, Tools: detail.ToolCalls, InputTokens: detail.TokensIn, OutputTokens: detail.TokensOut, CostMicros: detail.CostMicros, Currency: detail.Currency, ReportMode: detail.ReportMode, BudgetReason: detail.BudgetReason, Error: detail.Error, Report: detail.Report, ActualModels: []string{}}
	models := map[string]bool{}
	for _, step := range detail.Steps {
		if step.Kind == "llm" && step.Model != "" {
			models[step.Model] = true
			if step.Model != detail.Model {
				value.MixedModel = true
			}
		}
	}
	for model := range models {
		value.ActualModels = append(value.ActualModels, model)
	}
	sort.Strings(value.ActualModels)
	var parsed struct {
		RootCause struct {
			Component string `json:"component"`
		} `json:"root_cause"`
	}
	if json.Unmarshal(detail.Report, &parsed) == nil {
		value.Component = parsed.RootCause.Component
	}
	value.ComponentMatch = value.Component != "" && value.Component == expected
	// 未结束的任务没有最终耗时，不能用当前时间冒充结果。
	if detail.StartedAt != nil && detail.FinishedAt != nil {
		value.Seconds = detail.FinishedAt.Sub(*detail.StartedAt).Seconds()
	}
	return value
}
