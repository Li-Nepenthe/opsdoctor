// 本文件：从工具声明构造校验规则，并保留模型已经使用的公开 Schema 格式。
// 注册工具时直接读取声明字段并解析一次，以免公开 JSON 省略的零下界在校验时丢失。

package tools

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/jsonstrict"
	"github.com/google/jsonschema-go/jsonschema"
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

// parameterValidator 只保存已经解析的规则；实例参数不存进去，可以供并发调用共用。
type parameterValidator struct {
	resolved *jsonschema.Resolved // 初始化后只读，Validate 不重新解析 Schema。
	integers []string             // 工具读取的是 int，库解码与补默认值后需要转换。
}

// resolve 只用于注册表构建；定义来自代码，写错时启动失败，不能拖到第一次工具调用。
func (schema Schema) resolve() *parameterValidator {
	rule := &jsonschema.Schema{Type: schema.Type, Properties: map[string]*jsonschema.Schema{}, Required: schema.Required}
	if !schema.AdditionalProperties {
		rule.AdditionalProperties = &jsonschema.Schema{Not: &jsonschema.Schema{}}
	}
	validator := &parameterValidator{}
	// 必须读原始字段；Minimum 为 0 时，公开 JSON 为兼容旧提示词仍然会省略它。
	for name, property := range schema.Properties {
		rule.Properties[name] = property.rule()
		if property.Type == "integer" {
			validator.integers = append(validator.integers, name)
		}
	}
	resolved, err := rule.Resolve(&jsonschema.ResolveOptions{ValidateDefaults: true})
	if err != nil {
		panic(fmt.Errorf("工具参数定义不合法：%v", err))
	}
	validator.resolved = resolved
	return validator
}

// rule 只做声明转换，不再自行实现类型、范围、正则和枚举检查。
func (property Property) rule() *jsonschema.Schema {
	rule := &jsonschema.Schema{Type: property.Type, Description: property.Description, Pattern: property.Pattern}
	switch property.Type {
	case "integer":
		rule.Minimum, rule.Maximum = jsonschema.Ptr(float64(property.Minimum)), jsonschema.Ptr(float64(property.Maximum))
	case "string":
		rule.MinLength, rule.MaxLength = jsonschema.Ptr(property.MinLength), jsonschema.Ptr(property.MaxLength)
	default:
		panic("工具参数定义使用了不支持的类型")
	}
	// 空 Enum 表示没有枚举约束；不能转成库里的空数组，否则所有值都会被拒绝。
	for _, value := range property.Enum {
		rule.Enum = append(rule.Enum, value)
	}
	if property.Default != nil {
		raw, err := json.Marshal(property.Default)
		if err != nil {
			panic(fmt.Errorf("工具默认值无法编码：%v", err))
		}
		rule.Default = raw
	}
	return rule
}

// validate 保留原始输入边界，库只接收本次调用自己的 float64 参数树。
func (validator *parameterValidator) validate(raw json.RawMessage) (arguments, error) {
	if len(raw) == 0 || len(raw) > 8192 || !utf8.Valid(raw) {
		return nil, fmt.Errorf("参数必须是最多 8192 字节的 JSON 对象")
	}
	if _, err := jsonstrict.Decode(raw); err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("参数不符合工具定义：参数里的数字超出范围")
	}
	if err := validator.resolved.Validate(value); err != nil {
		return nil, fmt.Errorf("参数不符合工具定义：%v", err)
	}
	if err := validator.resolved.ApplyDefaults(&value); err != nil {
		return nil, fmt.Errorf("参数不符合工具定义：%v", err)
	}
	args := arguments(value.(map[string]any))
	// 先由库验证类型和范围，再转换。允许 3.0、1e3 等写法，工具实际使用的整数仍在范围内。
	for _, name := range validator.integers {
		if number, exists := args[name]; exists {
			args[name] = int(number.(float64))
		}
	}
	return args, nil
}
