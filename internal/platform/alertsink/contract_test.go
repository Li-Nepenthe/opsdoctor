// 本文件：用会在监听前失败的配置核对告警接收器自己的地址变量和令牌门槛。

package alertsink

import (
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
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

// TestAlertsinkConfig 用同一份部署变量读取逻辑核对默认地址、显式覆盖和令牌的字节下限。
func TestAlertsinkConfig(t *testing.T) {
	// 16 个双字节字符正好够 32 字节，能区分按字符数和按字节数校验。
	token := strings.Repeat("é", 16)
	for _, tc := range []struct {
		name     string
		values   map[string]string
		wantAddr string
		wantErr  string
	}{
		{"默认地址", map[string]string{"SHOP_ALERT_TOKEN": token}, ":8091", ""},
		{"显式地址", map[string]string{"ALERTSINK_ADDR": " 127.0.0.1:9900 ", "SHOP_ALERT_TOKEN": token}, "127.0.0.1:9900", ""},
		{"空白地址", map[string]string{"ALERTSINK_ADDR": " ", "SHOP_ALERT_TOKEN": token}, "", "环境变量 ALERTSINK_ADDR 不能为空"},
		{"缺少令牌", nil, "", "环境变量 SHOP_ALERT_TOKEN 不能为空"},
		{"令牌太短", map[string]string{"SHOP_ALERT_TOKEN": strings.Repeat("x", 31)}, "", "SHOP_ALERT_TOKEN 至少需要 32 字节"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := config.NewReader(func(name string) (string, bool) {
				value, ok := tc.values[name]
				return value, ok
			})
			settings, err := loadConfig(reader)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("期望 %q，得到 %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if settings.address != tc.wantAddr {
				t.Errorf("监听地址：得到 %q，期望 %q", settings.address, tc.wantAddr)
			}
			if settings.token != token {
				t.Error("读取的令牌与部署值不一致")
			}
		})
	}
}
