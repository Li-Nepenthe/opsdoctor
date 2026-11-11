// 本文件：逐字节核对模型收到的报告工具定义，包括工具说明、参数和组件枚举顺序。
// TestReportDefinitionGolden 固定组件表，并把编码结果直接与紧凑 JSON 对照。

package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// TestReportDefinitionGolden 使用固定顺序的组件表，枚举顺序变化同样会使测试失败。
func TestReportDefinitionGolden(t *testing.T) {
	components := []string{"unknown", "order-service", "inventory-service", "shop-mysql", "shop-redis", "kafka"}
	got, err := json.Marshal(reportDefinition(components))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/report_definition.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		// 只报告首个不同字节，避免长 Schema 淹没真正需要审核的位置。
		first := 0
		for first < min(len(got), len(want)) && got[first] == want[first] {
			first++
		}
		t.Fatalf("报告工具 JSON 在第 %d 字节发生变化：当前 %d 字节，基线 %d 字节；请先审核差异，不要自动更新 golden", first, len(got), len(want))
	}
}
