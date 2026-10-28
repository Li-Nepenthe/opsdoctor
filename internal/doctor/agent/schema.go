// 本文件：校验报告使用到的 JSON Schema 子集。
//
// 报告有嵌套对象和数组，阶段 5 的扁平工具参数校验不够用。这里仅实现报告真正
// 需要的类型、必填项、范围和枚举，公开 Schema 与运行时检查共用同一份结构。
//
// 主要内容：schema 描述规则；decodeJSON 拒绝重复字段；check 递归检查值。
// 阅读建议：先看 object 分支的未知字段检查，再看数组中的每条证据怎样被检查。

package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"slices"
	"unicode/utf8"
)

// schema 只用于本包拥有的报告格式，不是任意外部 Schema 的解释器。
type schema struct {
	Type        string             `json:"type"`                  // object、array、string、number、integer。
	Description string             `json:"description,omitempty"` // 供模型理解字段用途。
	Properties  map[string]*schema `json:"properties,omitempty"`  // 对象允许的字段。
	Required    []string           `json:"required,omitempty"`    // 必须显式出现，不能靠 Go 零值补齐。
	Additional  bool               `json:"additionalProperties"`  // 只有建议操作 args 可以有任意字段。
	Items       *schema            `json:"items,omitempty"`       // 数组的元素规则。
	Enum        []string           `json:"enum,omitempty"`        // 允许的组件或建议工具名。
	MinLength   int                `json:"minLength,omitempty"`   // 字符串最少字符数。
	MaxLength   int                `json:"maxLength,omitempty"`   // 字符串最多字符数。
	MaxItems    int                `json:"maxItems,omitempty"`    // 防止报告制造大量证据或建议。
	Minimum     *float64           `json:"minimum,omitempty"`     // 可选数值下界，零也是有效下界。
	Maximum     *float64           `json:"maximum,omitempty"`     // 可选数值上界。
}

// decodeJSON 用 Token 逐项读取，以便发现重复字段；普通 Unmarshal 会保留最后一次值。
func decodeJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := readValue(decoder, 0)
	if err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("只接受一份 JSON")
	}
	return value, nil
}

// readValue 限制嵌套层数，恶意 JSON 不能让递归无限深入。
func readValue(decoder *json.Decoder, depth int) (any, error) {
	// 只逐层接收对象与数组，结束符由当前递归层消费。
	if depth > 16 {
		return nil, fmt.Errorf("JSON 嵌套过深")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("JSON 在第 %d 字节附近格式错误：%v", decoder.InputOffset(), err)
	}
	switch token {
	case json.Delim('{'):
		value := map[string]any{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, fmt.Errorf("JSON 在第 %d 字节附近对象字段不合法：%v", decoder.InputOffset(), err)
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("对象字段名必须是字符串")
			}
			if _, exists := value[name]; exists {
				return nil, fmt.Errorf("字段 %s 重复", name)
			}
			item, err := readValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			value[name] = item
		}
		_, err = decoder.Token()
		return value, err
	case json.Delim('['):
		value := []any{}
		for decoder.More() {
			item, err := readValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			value = append(value, item)
		}
		_, err = decoder.Token()
		return value, err
	default:
		return token, nil
	}
}

// check 返回具体字段路径，让模型知道该改报告的哪一处。
func (rule *schema) check(value any, path string) error {
	// 结构校验先于业务校验，后面才能安全地读取 evidence.step。
	switch rule.Type {
	case "object":
		return rule.checkObject(value, path)
	case "array":
		items, ok := value.([]any)
		if !ok || len(items) > rule.MaxItems {
			return fmt.Errorf("%s 必须是最多 %d 项的数组", path, rule.MaxItems)
		}
		for index, item := range items {
			if err := rule.Items.check(item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case "string":
		text, ok := value.(string)
		if !ok || utf8.RuneCountInString(text) < rule.MinLength || utf8.RuneCountInString(text) > rule.MaxLength {
			return fmt.Errorf("%s 必须是 %d–%d 字符的字符串", path, rule.MinLength, rule.MaxLength)
		}
		if len(rule.Enum) > 0 && !slices.Contains(rule.Enum, text) {
			return fmt.Errorf("%s 不在允许的命名表中", path)
		}
	case "number", "integer":
		number, ok := value.(json.Number)
		if !ok {
			return fmt.Errorf("%s 必须是数字", path)
		}
		parsed, err := number.Float64()
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || (rule.Type == "integer" && parsed != math.Trunc(parsed)) || (rule.Minimum != nil && parsed < *rule.Minimum) || (rule.Maximum != nil && parsed > *rule.Maximum) {
			return fmt.Errorf("%s 的数字超出范围或不是要求的整数", path)
		}
	default:
		return fmt.Errorf("报告 Schema 含未支持的类型")
	}
	return nil
}

// checkObject 先检查必填字段，再检查多余字段，不把缺项悄悄当作空字符串。
func (rule *schema) checkObject(value any, path string) error {
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s 必须是对象", path)
	}
	for _, name := range rule.Required {
		if _, ok := object[name]; !ok {
			return fmt.Errorf("%s 缺少 %s", path, name)
		}
	}
	for name, item := range object {
		child, ok := rule.Properties[name]
		if !ok {
			if !rule.Additional {
				return fmt.Errorf("%s 包含未定义的字段 %s", path, name)
			}
			continue // 建议操作的 args 只记录；阶段 7 的写执行器另有严格校验。
		}
		if err := child.check(item, path+"."+name); err != nil {
			return err
		}
	}
	return nil
}
