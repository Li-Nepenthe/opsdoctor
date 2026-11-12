// 本文件：把模型看到的报告格式转成校验规则，供同一组件表的多次诊断复用。
// resolvedReportSchema 用完整组件数组作为缓存键，保留声明里的组件顺序。

package agent

import (
	"encoding/json"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
)

// schema 只用于本包拥有的报告格式，不是任意外部 Schema 的解释器。
type schema struct {
	Type        string             `json:"type"`
	Description string             `json:"description,omitempty"`
	Properties  map[string]*schema `json:"properties,omitempty"`
	Required    []string           `json:"required,omitempty"`   // 必须显式出现，不能靠 Go 零值补齐。
	Additional  bool               `json:"additionalProperties"` // 只有建议操作 args 可以有任意字段。
	Items       *schema            `json:"items,omitempty"`
	Enum        []string           `json:"enum,omitempty"` // 允许的组件或建议工具名。
	MinLength   int                `json:"minLength,omitempty"`
	MaxLength   int                `json:"maxLength,omitempty"`
	MaxItems    int                `json:"maxItems,omitempty"` // 防止报告制造大量证据或建议。
	Minimum     *float64           `json:"minimum,omitempty"`  // 可选数值下界，零也是有效下界。
	Maximum     *float64           `json:"maximum,omitempty"`
}

// reportValidator 把同一组件表的并发首次调用合在一起，解析失败也只记录一次。
type reportValidator struct {
	once sync.Once
	rule *jsonschema.Resolved // 初始化后只读，可安全用于多份报告。
	err  error                // 规则来自程序声明，出错不能改用宽松规则。
}

// reportValidators 的键来自部署配置中的组件表，值只缓存规则，不缓存任何报告或诊断数据。
var reportValidators sync.Map

// resolvedReportSchema 直接读取发给模型的同一份规则，组件的排列顺序也参与缓存键。
func resolvedReportSchema(components []string) (*jsonschema.Resolved, error) {
	key, _ := json.Marshal(components)
	stored, _ := reportValidators.LoadOrStore(string(key), &reportValidator{})
	validator := stored.(*reportValidator)
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
