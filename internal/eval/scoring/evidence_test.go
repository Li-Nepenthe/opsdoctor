// 本文件：用伪造的工具记录验证评分不会从模型自述中猜证据。
//
// 主要内容：TestEvidence 覆盖完整表格、截断和失败；TestComponentAndCoverage 核对分母与精确比较。
// 阅读建议：所有数据都在内存中，测试不连接靶场或模型。

package scoring

import (
	"encoding/json"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// TestEvidence 对相同表格逐项改变可信条件，防止只看一段包含 ALL 的文字就给分。
func TestEvidence(t *testing.T) {
	for _, test := range []struct {
		name, status, kind       string
		truncated, limited, want bool
	}{
		{"真实全表扫描", "ok", "tool", false, false, true},     // 完整成功记录才支持这个事实。
		{"结果被截断", "ok", "tool", true, false, false},      // 不完整内容不作全量判断。
		{"后端限制行数", "ok", "tool", false, true, false},     // 工具没有二次截断也可能已被后端截断。
		{"工具失败", "timeout", "tool", false, false, false}, // 超时文字不算执行计划。
		{"模型自述", "ok", "llm", false, false, false},       // 报告声称扫描不能替代工具。
	} {
		t.Run(test.name, func(t *testing.T) {
			data, _ := json.Marshal(source.Table{Columns: []string{"type", "table"}, Rows: [][]string{{"ALL", "stock"}}, Limited: test.limited})
			raw, _ := json.Marshal(tools.Result{Status: test.status, Data: data, Truncated: test.truncated})
			step := agent.Step{Seq: 4, Kind: test.kind, ToolName: "mysql_explain", Status: test.status, Result: string(raw)}
			got := Evidence(Rule{Milestone: 1, Kind: "full_scan", Table: "stock"}, []agent.Step{step})
			if got.Achieved != test.want || got.Achieved && (len(got.Steps) != 1 || got.Steps[0] != 4) {
				t.Fatalf("错误证据：%+v", got)
			}
		})
	}
	// 列名缺失不是“查到零行”；正确空表可以证明没有目标索引。
	if missingIndex(source.Table{}, "wanted") || !missingIndex(source.Table{Columns: []string{"INDEX_NAME"}}, "wanted") || missingIndex(source.Table{Columns: []string{"INDEX_NAME"}, Rows: [][]string{{"wanted"}}}, "wanted") {
		t.Fatal("索引存在性判断错误")
	}
}

// TestComponentAndCoverage 确认名字相近不能得分，未取得的证据仍计入分母。
func TestComponentAndCoverage(t *testing.T) {
	if Component("stock", "mysql:shop.stock") || Component("", "") || !Component("none", "none") {
		t.Fatal("组件比较不精确")
	}
	if Coverage([]EvidenceResult{{Achieved: true}, {Achieved: false}}) != 0.5 || Coverage(nil) != 0 {
		t.Fatal("覆盖率分母错误")
	}
}
