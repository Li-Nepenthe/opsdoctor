// 本文件：用会在监听前失败的配置核对告警接收器自己的地址变量和令牌门槛。

package alertsink

import (
	"strings"
	"testing"
)

// TestAlertsinkEnvironmentContract 验证两个变量缺失和短令牌的错误，所有用例都在创建服务器前结束。
func TestAlertsinkEnvironmentContract(t *testing.T) {
	for _, tc := range []struct{ addr, token, want string }{
		{"", "", "环境变量 ALERTSINK_ADDR 不能为空\n环境变量 SHOP_ALERT_TOKEN 不能为空"},
		{":0", strings.Repeat("x", 31), "SHOP_ALERT_TOKEN 至少需要 32 字节"},
	} {
		t.Setenv("ALERTSINK_ADDR", tc.addr)
		t.Setenv("SHOP_ALERT_TOKEN", tc.token)
		if err := Run(t.Context()); err == nil || err.Error() != tc.want {
			t.Fatalf("接收器配置：%v", err)
		}
	}
}
