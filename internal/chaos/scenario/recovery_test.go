// 本文件：在医生之外，核对 shop 的恢复观察期是否覆盖告警发出的等待时间。
//
// 不同告警需要持续不同时间才会发出。前一条已经恢复，后一条还可能在等待。
// 这里读取部署文件计算下界，让以后延长告警 for 的人同时更新系统接入配置。
//
// 主要内容：TestShopRecoveryWindow 检查最长 for、规则评估间隔和分组等待时间之和。
// 阅读建议：测试只读取配置，不向医生传递规则或实验答案，也不启动真实服务。

package scenario

import (
	"os"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
)

// TestShopRecoveryWindow 防止恢复观察期比仍在等待发出的告警更短。
func TestShopRecoveryWindow(t *testing.T) {
	// 只声明需要比较的字段，部署文件里的抓取端点和推送凭据不参与测试输出。
	var rules struct {
		Groups []struct {
			Interval string `yaml:"interval"`
			Rules    []struct {
				For string `yaml:"for"`
			} `yaml:"rules"`
		} `yaml:"groups"`
	}
	var prom struct {
		Global struct {
			Evaluation string `yaml:"evaluation_interval"`
		} `yaml:"global"`
	}
	var manager struct {
		Route struct {
			Wait string `yaml:"group_wait"`
		} `yaml:"route"`
	}
	var target struct {
		Window string `yaml:"alert_recovery_window"`
	}
	for path, value := range map[string]any{
		"../../../deploy/prometheus/rules/shop.yml":     &rules,
		"../../../deploy/prometheus/prometheus.yml":     &prom,
		"../../../deploy/alertmanager/alertmanager.yml": &manager,
		"../../../deploy/doctor/targets/shop.yaml":      &target,
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := yaml.Unmarshal(data, value); err != nil {
			t.Fatal(err)
		}
	}
	parse := func(text string) time.Duration {
		t.Helper()
		value, err := time.ParseDuration(text)
		if err != nil || value < 0 {
			t.Fatalf("无效时长 %q", text)
		}
		return value
	}
	evaluation, wait := parse(prom.Global.Evaluation), parse(manager.Route.Wait)
	var required time.Duration
	for _, group := range rules.Groups {
		interval := evaluation
		if group.Interval != "" {
			interval = parse(group.Interval)
		}
		for _, rule := range group.Rules {
			// 没有 for 的规则立即判定；仍然需要一次评估和一次通知等待。
			var pending time.Duration
			if rule.For != "" {
				pending = parse(rule.For)
			}
			required = max(required, pending+interval+wait)
		}
	}
	if required == 0 {
		t.Fatal("没有读到 shop 告警规则")
	}
	if actual := parse(target.Window); actual < required {
		t.Fatalf("shop 观察期 %s 短于最长 for + 评估间隔 + group_wait = %s，请同步 shop.yaml", actual, required)
	}
	t.Logf("shop 观察期 %s；规则所需下界 %s", target.Window, required)
}
