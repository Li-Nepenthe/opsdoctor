// 本文件：验证命令行参数不会被误解或截断。
//
// TestParseStock 在建立网络连接前检查缺商品 ID、负件数、int32 溢出和多余参数。
// 先看每行用例解释，再对照 parseStock 的边界。实际 RPC 由库存包测试和 Compose 冒烟覆盖。
//
// 主要内容：
//   - TestParseStock：在无网络环境检查 CLI 边界，尤其防止 int64 到 int32 的静默截断。

package shopctl

import (
	"io"
	"testing"
)

// TestParseStock 在无网络环境检查 CLI 边界，尤其防止 int64 到 int32 的静默截断。
func TestParseStock(t *testing.T) {
	cases := []struct {
		name  string   // name 描述用户输入代表的场景。
		args  []string // args 是去掉 stock 前缀后的参数。
		valid bool     // valid 为 false 时不能创建 gRPC 客户端。
	}{
		{"合法查询", []string{"get", "--sku", "1"}, true},                                               // 最小查询命令。
		{"合法扣减", []string{"deduct", "--sku", "1", "--quantity", "2"}, true},                         // 显式件数。
		{"合法补库存", []string{"restock", "--sku", "1", "--quantity", "10000", "--count", "100"}, true}, // 明确范围才能批量增加。
		{"扣减不许批量", []string{"deduct", "--sku", "1", "--count", "2"}, false},                         // 不扩大原扣减命令的范围。
		{"补库存范围过大", []string{"restock", "--sku", "1", "--count", "10001"}, false},                   // 防止误操作覆盖整库。
		{"商品编号溢出", []string{"restock", "--sku", "9223372036854775807", "--count", "2"}, false},      // 连续范围也要检查整数上界。
		{"缺少子命令", nil, false},                                                         // 不能猜测用户是查还是扣。
		{"缺少商品", []string{"get"}, false},                                              // 缺少必填 ID。
		{"负件数", []string{"deduct", "--sku", "1", "--quantity", "-1"}, false},          // 不允许负扣减。
		{"整数溢出", []string{"deduct", "--sku", "1", "--quantity", "2147483648"}, false}, // 必须在类型转换前拒绝。
		{"零超时", []string{"get", "--sku", "1", "--timeout", "0s"}, false},              // 避免请求还没发出就结束。
		{"多余参数", []string{"get", "--sku", "1", "extra"}, false},                       // 防止用户误以为 extra 已生效。
	}
	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			_, err := parseStock(testcase.args, io.Discard)
			if (err == nil) != testcase.valid {
				t.Fatalf("参数校验：%v", err)
			}
		})
	}
}
