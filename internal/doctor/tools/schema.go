// 本文件：用同一份字段描述生成 JSON Schema，并校验实际工具参数。
//
// 当前只读工具只需要字符串和整数。这里实现这两个类型的明确规则，不假装支持
// 任意 JSON Schema。未知字段、重复字段、对象套对象都会被拒绝。
//
// 主要内容：
//   - Schema、Property：工具公开的参数说明。
//   - validate：解析单个对象并检查必填、类型、范围和允许值。
//   - arguments：让工具按已经验证的类型取值。
//
// 阅读建议：先看未知字段为什么立即拒绝，再看默认值如何只由注册表提供。

package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"unicode/utf8"
)

// Schema 只允许定义过的顶层字段，工具参数中从来没有系统名、URL 或主机。
type Schema struct {
	Type                 string              `json:"type"`                 // 固定 object。
	Properties           map[string]Property `json:"properties"`           // 每个名字与说明都由代码定义。
	Required             []string            `json:"required"`             // 必填字段清单。
	AdditionalProperties bool                `json:"additionalProperties"` // 始终 false。
}

// Property 的限制同时用于对外说明和运行时校验，避免文档与行为各写一套。
type Property struct {
	Type        string   `json:"type"`                // string 或 integer。
	Description string   `json:"description"`         // 写清楚单位和用途。
	Minimum     int      `json:"minimum,omitempty"`   // 整数下限。
	Maximum     int      `json:"maximum,omitempty"`   // 整数上限。
	MinLength   int      `json:"minLength,omitempty"` // 字符串最少字符数。
	MaxLength   int      `json:"maxLength,omitempty"` // 字符串最多字符数。
	Pattern     string   `json:"pattern,omitempty"`   // 服务名、表名等简单语法。
	Enum        []string `json:"enum,omitempty"`      // 固定选项，如日志级别。
	Default     any      `json:"default,omitempty"`   // 缺省值由工具定义，调用者不能重写规则。
}

// arguments 只在校验成功后交给工具，取值时无需重复做类型猜测。
type arguments map[string]any

// text 读取可选字符串；未提供的字段使用空值。
func (args arguments) text(name string) string { value, _ := args[name].(string); return value }

// integer 读取整数，默认值也已由校验器转换成 int。
func (args arguments) integer(name string) int { value, _ := args[name].(int); return value }

// validate 逐个读取字段，才能拒绝 encoding/json 默认会悄悄覆盖的重复键。
func (schema Schema) validate(raw json.RawMessage) (arguments, error) {
	// 逐个读字段才能发现重复键；直接解码进 map 会丢失前一次出现的信息。
	if len(raw) == 0 || len(raw) > 8192 || !utf8.Valid(raw) {
		return nil, fmt.Errorf("参数必须是最多 8192 字节的 JSON 对象")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return nil, fmt.Errorf("工具参数必须是一个 JSON 对象")
	}
	args := make(arguments)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("参数 JSON 格式错误")
		}
		name, ok := key.(string)
		if !ok {
			return nil, fmt.Errorf("参数名必须是字符串")
		}
		property, ok := schema.Properties[name]
		if !ok {
			return nil, fmt.Errorf("包含工具未定义的字段；不接受 URL、主机或系统参数")
		}
		if _, exists := args[name]; exists {
			return nil, fmt.Errorf("字段 %s 重复", name)
		}
		var value any
		if decoder.Decode(&value) != nil {
			return nil, fmt.Errorf("参数 JSON 格式错误")
		}
		checked, err := property.check(value)
		if err != nil {
			return nil, fmt.Errorf("字段 %s：%s", name, err)
		}
		args[name] = checked
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("参数 JSON 未结束")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("只接受一个 JSON 对象")
	}
	return schema.complete(args)
}

// complete 在格式校验后检查必填字段，缺省值只能取自工具定义。
func (schema Schema) complete(args arguments) (arguments, error) {
	for name, property := range schema.Properties {
		if _, exists := args[name]; exists {
			continue
		}
		if slices.Contains(schema.Required, name) {
			return nil, fmt.Errorf("缺少必填字段 %s", name)
		}
		if property.Default != nil {
			args[name] = property.Default
		}
	}
	return args, nil
}

// check 不做宽松类型转换。字符串 "10" 和数字 10 不是同一种工具参数。
func (property Property) check(value any) (any, error) {
	// 只接受声明的类型。把数字字符串自动转成整数，会掩盖调用者提供错参数的问题。
	switch property.Type {
	case "string":
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("必须是字符串")
		}
		length := utf8.RuneCountInString(text)
		if length < property.MinLength || length > property.MaxLength {
			return nil, fmt.Errorf("字符数必须在 %d–%d 之间", property.MinLength, property.MaxLength)
		}
		if property.Pattern != "" && !regexp.MustCompile(property.Pattern).MatchString(text) {
			return nil, fmt.Errorf("字符串格式不符合要求")
		}
		if len(property.Enum) > 0 && !slices.Contains(property.Enum, text) {
			return nil, fmt.Errorf("不在允许的选项中")
		}
		return text, nil
	case "integer":
		number, ok := value.(json.Number)
		if !ok {
			return nil, fmt.Errorf("必须是整数")
		}
		integer, err := number.Int64()
		if err != nil || integer < int64(property.Minimum) || integer > int64(property.Maximum) {
			return nil, fmt.Errorf("整数必须在 %d–%d 之间", property.Minimum, property.Maximum)
		}
		return int(integer), nil
	default:
		return nil, fmt.Errorf("工具参数定义使用了不支持的类型")
	}
}
