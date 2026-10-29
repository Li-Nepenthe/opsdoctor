// 本文件：用请求审计的固定状态验证哪些失败可以补跑。
//
// 主要内容：TestInfrastructureFailure 检查一次有效响应也没有的边界。
// 阅读建议：失败原因相似不代表处理相同，测试必须保留预算、格式和人为取消的区别。

package store

import "testing"

// TestInfrastructureFailure 不依赖错误文案，也不会连接真实模型。
func TestInfrastructureFailure(t *testing.T) {
	for _, test := range []struct {
		name, state string
		report      bool
		attempts    []string
		want        bool
	}{
		{"网络尝试全失败", "failed", false, []string{"network_error", "network_error"}, true}, // 没有一次回答，可补跑一次。
		{"上游暂不可用", "failed", false, []string{"http_503", "http_429"}, true},            // 限流也是可观测的可靠性失败。
		{"回答后才断网", "failed", false, []string{"ok", "network_error"}, false},            // 已开始推理，不能从正确率分母排除。
		{"格式错误", "failed", false, []string{"http_200"}, false},                         // 收到损坏回复仍保留格式失败。
		{"鉴权错误", "failed", false, []string{"http_401"}, false},                         // 配置错误要修配置，不能反复收费试跑。
		{"余额不足", "failed", false, []string{"http_402"}, false},                         // 余额不是可靠性豁免。
		{"预算结束", "budget_exceeded", false, []string{"network_error"}, false},           // 用完预算不能换槽位绕过。
		{"主动取消", "cancelled", false, []string{"cancelled"}, false},                     // 用户取消不是自动重跑许可。
		{"尚未结算", "failed", false, []string{"reserved"}, false},                         // 未知状态先核对，不能猜测供应商结果。
		{"有报告", "failed", true, []string{"network_error"}, false},                      // 已有报告就按实际报告评分。
		{"没发请求", "failed", false, nil, false},                                          // 本地错误不能冒充网络请求失败。
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := infrastructureFailure(test.state, test.report, test.attempts); got != test.want {
				t.Fatalf("可补跑=%t，期望 %t", got, test.want)
			}
		})
	}
}
