// 本文件：验证新环境不会因为填了 Key 就自动花钱，以及所有预算配置的边界。
//
// 配置测试注入内存变量，不读取本机 .env。即使测试了 openai 分支，也只解析参数，
// 不构造真实 HTTP 请求。
//
// 主要内容：TestAgentDefaults 检查免费默认值；TestAgentConfigRejects 检查越界配置；
// TestKnownSecrets 检查只有连接串时也能清理密码。
// 阅读建议：空值和未知 provider 都要失败，不能静默使用另一种模型。

package app

import (
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestKnownSecrets 模拟容器只收到 DSN 和 URL，证明两种连接串里的密码都被收集。
func TestKnownSecrets(t *testing.T) {
	t.Setenv("SHOP_MYSQL_RO_DSN", "diag_ro:demo-mysql-secret@tcp(localhost:3306)/shop")
	t.Setenv("SHOP_REDIS_RO_URL", "redis://diag_ro:demo-redis-secret@localhost:6379/0")
	redactor := llm.NewRedactor(knownSecrets()...)
	clean := redactor.Text("demo-mysql-secret demo-redis-secret")
	if strings.Contains(clean, "demo-") {
		t.Fatal("连接串中的密码没有被清理")
	}
}

// TestAgentDefaults 明确证明 Mock 配置不读取模型 Key；测试用读取器遇到它就失败。
func TestAgentDefaults(t *testing.T) {
	value, err := loadAgentConfig(config.NewReader(func(name string) (string, bool) {
		if name == "LLM_API_KEY" {
			t.Fatal("Mock 读取了 Key")
		}
		return "", false
	}))
	if err != nil || value.provider != "mock" || value.model != "mock-script-v1" || value.concurrent != 2 || value.perTarget != 1 || value.totalMicros != 10_000_000 || value.budgetAccount != "phase-07-08" {
		t.Fatalf("默认配置错误：%v", err)
	}
}

// TestAgentConfigRejects 每个用例只改一个边界，断言启动拒绝而不是退回默认值。
func TestAgentConfigRejects(t *testing.T) {
	for _, testcase := range []struct{ name, value string }{
		{"LLM_PROVIDER", ""},                      // 显式空值通常是部署变量缺失。
		{"LLM_PROVIDER", "deepseek"},              // 必须显式使用协议实现 openai。
		{"LLM_PROVIDER", "openai"},                // 缺地址、模型、Key 和单价，不能启动真实客户端。
		{"LLM_TOTAL_BUDGET_CNY", "0"},             // 零或负数不是关闭预算检查的开关。
		{"DOCTOR_MAX_CONCURRENT", "3"},            // 本阶段全局最多两个。
		{"DOCTOR_MAX_CONCURRENT_PER_TARGET", "2"}, // 单系统只能一个。
		{"LLM_TIMEOUT", "3m"},                     // 单轮请求不能无限占住五分钟总预算。
		{"DOCTOR_ALERT_PENDING_MAX_AGE", "0s"},    // 过期线必须明确为正。
		{"DOCTOR_ALERT_SILENCE_TIMEOUT", "1m"},    // 不能短于正常的告警重发间隔。
	} {
		t.Run(testcase.name+testcase.value, func(t *testing.T) {
			_, err := loadAgentConfig(config.NewReader(func(name string) (string, bool) { return testcase.value, name == testcase.name }))
			if err == nil {
				t.Fatal("错误配置被接受")
			}
		})
	}
}
