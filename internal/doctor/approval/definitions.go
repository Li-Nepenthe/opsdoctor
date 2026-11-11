// 本文件：列出当前系统允许模型申请的写操作，以及人审阅时需要的理由、证据、预期效果和风险。
// 申请里没有目标系统、地址、令牌或批准开关。程序校验参数并生成计划后，由人决定是否批准。

package approval

import (
	"encoding/json"
	"sort"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// definitions 只在有对应资源时列出工具；MCP 使用另一份只读菜单，拿不到这些申请入口。
func (backend *Backend) definitions() []llm.Tool {
	// 没有配置某种资源，就不向模型承诺有这项能力。菜单排序固定，便于前缀缓存。
	values := []llm.Tool{}
	if backend.ddl != nil {
		tables := []string{}
		for name := range backend.tables {
			tables = append(tables, name)
		}
		sort.Strings(tables)
		values = append(values, writeDefinition("create_index", "申请在允许的表上创建最多三列索引。程序生成 idx_doctor_ 索引名与确切语句，必须等人批准。请求取消后 MySQL 的 DDL 可能仍继续。", map[string]any{"table": map[string]any{"type": "string", "enum": tables}, "columns": map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "minItems": 1, "maxItems": 3, "uniqueItems": true}, "unique": map[string]string{"type": "boolean"}}, []string{"table", "columns", "unique"}))
	}
	if len(backend.ops.Services) > 0 {
		services := []string{}
		for name := range backend.ops.Services {
			services = append(services, name)
		}
		sort.Strings(services)
		values = append(values, writeDefinition("set_db_pool_size", "申请临时调整某服务的数据库池上限；仅止血，重启恢复配置。必须等人批准。", map[string]any{"service": map[string]any{"type": "string", "enum": services}, "size": map[string]any{"type": "integer", "minimum": backend.ops.PoolMin, "maximum": backend.ops.PoolMax}}, []string{"service", "size"}))
	}
	if backend.ops.WarmService != "" {
		values = append(values, writeDefinition("warm_cache", "申请预热最多一百个商品，只补缺失缓存，不修改业务数据。必须等人批准。", map[string]any{"ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer", "minimum": 1}, "minItems": 1, "maxItems": 100, "uniqueItems": true}}, []string{"ids"}))
	}
	return values
}

// writeDefinition 把证据约束说明在菜单里，但真正的步骤存在性检查仍在 prepare 中。
func writeDefinition(name, description string, properties map[string]any, required []string) llm.Tool {
	for _, field := range []string{"reason", "expected_effect", "risk"} {
		properties[field] = map[string]any{"type": "string", "minLength": 1, "maxLength": 2048}
		required = append(required, field)
	}
	properties["evidence_steps"] = map[string]any{"type": "array", "items": map[string]any{"type": "integer", "minimum": 1}, "minItems": 1, "maxItems": 10, "uniqueItems": true}
	required = append(required, "evidence_steps")
	parameters, _ := json.Marshal(map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false})
	return llm.Tool{Type: "function", Function: llm.ToolDefinition{Name: name, Description: description, Parameters: parameters}}
}
