// 本文件：固定报告工具的完整定义，防止校验器替换改变模型看到的报告格式。
//
// 组件表直接写在测试中，不从配置或环境读取。golden 保存原始紧凑 JSON，
// 比较时不忽略空白，也不把 JSON 解成对象后做语义比较。
//
// 主要内容：TestReportDefinitionGolden 检查工具外壳、描述和 parameters 的全部字节。
// 阅读建议：这份基线属于项目输出，不依赖候选校验库的行为。

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
