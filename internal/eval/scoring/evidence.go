// 本文件：依据真实工具返回值判断可以机械核对的证据。
//
// 规则只证明一个有限事实。例如索引列表必须完整，才能判断某个索引不存在。
// 没调用工具、工具失败和结果截断都算没拿到这项证据，不能从最终报告倒推。
//
// 主要内容：Component 精确比较；Evidence 执行规则；Coverage 计算覆盖率；
// fullScan、missingIndex 读取 MySQL 表格而不是在整段对话里搜索关键词。
// 阅读建议：规则返回步骤号，人工校准时可以回到那一步复核。

package scoring

import (
	"encoding/json"
	"strings"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// Rule 由评测场景声明，医生的工具和提示都读不到这些标准答案。
type Rule struct {
	Milestone int    `json:"milestone" yaml:"milestone"`             // 从一开始，对应 evidence_milestones。
	Kind      string `json:"kind" yaml:"kind"`                       // full_scan 或 missing_index。
	Table     string `json:"table" yaml:"table"`                     // 要核对的表，不能只看另一个表恰好扫描了。
	Index     string `json:"index,omitempty" yaml:"index,omitempty"` // missing_index 要找的索引名。
}

// EvidenceResult 保留判断方式和引用，后续评委不得覆盖规则已经确定的结果。
type EvidenceResult struct {
	Milestone int    `json:"milestone"` // 证据项目号。
	Achieved  bool   `json:"achieved"`  // 是否由实际记录支持。
	Method    string `json:"method"`    // rule 或 judge。
	Steps     []int  `json:"steps"`     // 真实步骤号，没有证据时为空。
	Reason    string `json:"reason"`    // 给人复核的简短依据。
}

// Component 不忽略大小写、不接受子串，保证不同模型遵循同一份组件命名表。
func Component(actual, expected string) bool { return actual != "" && actual == expected }

// Evidence 只检查成功且完整的表格。自然语言说“没有索引”不会命中这里。
func Evidence(rule Rule, steps []agent.Step) EvidenceResult {
	value := EvidenceResult{Milestone: rule.Milestone, Method: "rule", Steps: []int{}, Reason: "未取得满足规则的完整工具结果"}
	// 三道检查分别排除模型自述、工具失败和内容不完整；不能只看一段关键词。
	for _, step := range steps {
		if step.Kind != "tool" || step.Status != "ok" {
			continue
		}
		var result tools.Result
		var table source.Table
		if json.Unmarshal([]byte(step.Result), &result) != nil || result.Status != "ok" || result.Truncated || json.Unmarshal(result.Data, &table) != nil || table.Limited {
			continue
		}
		matched := false
		switch rule.Kind {
		case "full_scan":
			matched = step.ToolName == "mysql_explain" && fullScan(table, rule.Table)
		case "missing_index":
			var args struct {
				Table string `json:"table"`
			}
			matched = step.ToolName == "mysql_show_indexes" && json.Unmarshal(step.Args, &args) == nil && args.Table == rule.Table && missingIndex(table, rule.Index)
		}
		if matched {
			value.Achieved, value.Reason = true, "完整工具表格满足已冻结的规则"
			value.Steps = append(value.Steps, step.Seq)
		}
	}
	return value
}

// fullScan 按列名查找，不依赖 MySQL 不同版本的列顺序；只接受真实表名和 ALL。
func fullScan(table source.Table, name string) bool {
	nameColumn, typeColumn := column(table.Columns, "table"), column(table.Columns, "type")
	if nameColumn < 0 || typeColumn < 0 {
		return false
	}
	for _, row := range table.Rows {
		if len(row) > max(nameColumn, typeColumn) && row[nameColumn] == name && row[typeColumn] == "ALL" {
			return true
		}
	}
	return false
}

// missingIndex 必须拿到正确列，不能把格式错误或截断后的空列表误认成“没有索引”。
func missingIndex(table source.Table, name string) bool {
	index := column(table.Columns, "INDEX_NAME")
	if index < 0 || name == "" {
		return false
	}
	for _, row := range table.Rows {
		if len(row) <= index || row[index] == name {
			return false
		}
	}
	return true
}

// column 只对元数据列名忽略大小写，业务表名和索引名仍精确比较。
func column(columns []string, name string) int {
	for index, value := range columns {
		if strings.EqualFold(value, name) {
			return index
		}
	}
	return -1
}

// Coverage 的分母是全部里程碑，不能因为没查询某项就把它从分母删除。
func Coverage(values []EvidenceResult) float64 {
	if len(values) == 0 {
		return 0
	}
	count := 0
	for _, value := range values {
		if value.Achieved {
			count++
		}
	}
	return float64(count) / float64(len(values))
}
