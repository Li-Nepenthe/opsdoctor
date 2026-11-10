// 本文件：保留报告 Schema 的公开格式，按组件表缓存 jsonschema-go 的校验规则。
//
// 模型看到的声明结构不能因换库而变化。规则由同一份 JSON 转成库的 Schema，
// 每种组件表只 Resolve 一次；字段类型、必填、范围和枚举都交给库检查。
//
// 主要内容：schema 保持原 JSON 声明；resolvedReportSchema 安全复用已经解析的规则。
// 阅读建议：组件顺序也是模型输入的一部分，缓存键使用完整数组 JSON，不能简单拼名字。

package agent

import (
	"encoding/json"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
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

// reportValidator 把同一组件表的并发首次调用合在一起，解析失败也只记录一次。
type reportValidator struct {
	once sync.Once            // 只允许一次 Resolve，其他调用等待同一个结果。
	rule *jsonschema.Resolved // 初始化后只读，可安全用于多份报告。
	err  error                // 规则来自程序声明，出错不能改用宽松规则。
}

// reportValidators 的键来自部署配置中的组件表，值只缓存规则，不缓存任何报告或诊断数据。
var reportValidators sync.Map

// resolvedReportSchema 保留声明的字段顺序和组件顺序，不改变 reportDefinition 的编码路径。
func resolvedReportSchema(components []string) (*jsonschema.Resolved, error) {
	key, _ := json.Marshal(components)
	stored, _ := reportValidators.LoadOrStore(string(key), &reportValidator{})
	validator := stored.(*reportValidator)
	// 多个诊断同时遇到同一组件表时，也只做一次规则解析。
	validator.once.Do(func() {
		raw, _ := json.Marshal(reportSchema(components))
		var rule jsonschema.Schema
		if err := json.Unmarshal(raw, &rule); err != nil {
			validator.err = err
			return
		}
		validator.rule, validator.err = rule.Resolve(nil)
	})
	return validator.rule, validator.err
}
