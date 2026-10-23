// 本文件：验证核对命令不会把非零差值报告成成功。
//
// 这里只测试输出和退出结果，真实汇总 SQL 在 sales 的集成测试中验证。
//
// 主要内容：
//   - TestCheckOutput：分别检查一致、销量不足和销量过多。
//
// 阅读建议：负差值也必须失败，不能只检查“销量少了”的情况。

package shopctl

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/shop/sales"
)

// TestCheckOutput 使用明确的总量，让输出必须来自传入结果，而不是固定写零。
func TestCheckOutput(t *testing.T) {
	for _, testcase := range []struct {
		name      string        // 描述两个总量的关系。
		summary   sales.Summary // 真实查询会返回同样的结构。
		wantError bool          // 只有零差值可作为通过。
	}{
		{"一致", sales.Summary{Ordered: 10, Sold: 10}, false},          // 相等时仍说明总量检查的局限。
		{"少统计", sales.Summary{Ordered: 10, Sold: 8, Gap: 2}, true},   // 可能是消费延迟或丢消息。
		{"多统计", sales.Summary{Ordered: 10, Sold: 12, Gap: -2}, true}, // 可能有重复统计或人工修改。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			var output bytes.Buffer
			err := writeCheck(&output, testcase.summary)
			if (err != nil) != testcase.wantError || !strings.Contains(output.String(), "ordered_quantity=10") {
				t.Fatalf("输出=%s err=%v", output.String(), err)
			}
		})
	}
}
