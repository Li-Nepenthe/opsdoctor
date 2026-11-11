// 本文件：向模型声明报告格式，收到报告后核对组件、建议和证据。
// reportSchema 与 validateReport 共用规则，证据编号还须来自本次诊断。报告中的建议只供记录。

package agent

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/jsonstrict"
)

// reportSchema 对象字段全部显式必填，unknowns 可为空，但不能省略。
func reportSchema(components []string) *schema {
	zero, one := 0.0, 1.0
	text := func(minimum, maximum int) *schema {
		return &schema{Type: "string", MinLength: minimum, MaxLength: maximum}
	}
	object := func(properties map[string]*schema) *schema {
		required := make([]string, 0, len(properties))
		// 固定字段次序由下面构造后排序，保证工具前缀不会因 map 遍历变化。
		for name := range properties {
			required = append(required, name)
		}
		slices.Sort(required)
		return &schema{Type: "object", Properties: properties, Required: required}
	}
	component := text(1, 128)
	component.Enum = components
	actionName := text(1, 32)
	actionName.Enum = []string{"create_index", "set_db_pool_size", "warm_cache", "manual"}
	return object(map[string]*schema{
		"summary":           text(1, 1000),
		"root_cause":        object(map[string]*schema{"component": component, "mechanism": text(1, 2000), "confidence": {Type: "number", Minimum: &zero, Maximum: &one}}),
		"evidence":          {Type: "array", MaxItems: 40, Items: object(map[string]*schema{"step": {Type: "integer", Minimum: &one}, "observation": text(1, 1000)})},
		"suggested_actions": {Type: "array", MaxItems: 10, Items: object(map[string]*schema{"tool": actionName, "args": {Type: "object", Additional: true}, "risk": text(1, 1000)})},
		"unknowns":          text(0, 2000),
	})
}

// reportDefinition 让模型看到的 Schema 与 validateReport 使用的 Schema 来自同一函数。
func reportDefinition(components []string) llm.Tool {
	raw, _ := json.Marshal(reportSchema(components))
	return llm.Tool{Type: "function", Function: llm.ToolDefinition{Name: "submit_report", Description: "单独提交最终报告。evidence.step 必须是本次诊断已经保存的真实步骤；建议操作只记录不执行。", Parameters: raw}}
}

// validateReport 用 float64 副本校验，用保留 json.Number 的树输出，校验不能悄悄修改报告数字。
func validateReport(raw []byte, components []string, existing map[int]bool) (json.RawMessage, error) {
	const maximumReportBytes = 16 << 10 // 足以容纳多条证据，仍避免数据库和页面接收无限文本。
	if len(raw) > maximumReportBytes {
		return nil, fmt.Errorf("报告超过 16 KiB，请缩短描述")
	}
	original, err := jsonstrict.Decode(raw)
	if err != nil {
		return nil, err
	}
	// 第二遍为校验库准备数值，输出仍取第一遍保留的数字原文。两次解码都沿用 JSON 对非法 UTF-8 的替换。
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("报告里的数字超出范围")
	}
	rule, err := resolvedReportSchema(components)
	if err != nil {
		return nil, fmt.Errorf("报告格式定义无法解析")
	}
	if err := rule.Validate(value); err != nil {
		return nil, fmt.Errorf("报告不符合格式：%v", err)
	}
	object := value.(map[string]any)
	for _, item := range object["evidence"].([]any) {
		// 外形已验证，这里只检查编号是否属于本次保存过的步骤，不能借用别的诊断。
		step := item.(map[string]any)["step"].(float64)
		if !existing[int(step)] {
			return nil, fmt.Errorf("证据步骤 %g 在本次诊断中不存在，请引用工具结果里的 step", step)
		}
	}
	return json.Marshal(original)
}
