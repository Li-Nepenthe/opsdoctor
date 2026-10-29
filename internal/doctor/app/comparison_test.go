// 本文件：验证第二家模型只有显式启用时才读取凭据。
//
// 主要内容：TestComparisonConfig 检查免费总开关、独立账户、缺 Key 和费用范围。
// 阅读建议：所有配置都来自内存字典，示例 Key 是假值；不会访问网络。

package app

import (
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestComparisonConfig 防止填了 Key 就隐式付费，也防止美元与人民币共用账户名。
func TestComparisonConfig(t *testing.T) {
	if value, err := loadComparison(config.NewReader(func(string) (string, bool) {
		t.Fatal("Mock 不该读取任何对比配置")
		return "", false
	}), agentSettings{provider: "mock"}); err != nil || value != nil {
		t.Fatal("Mock 装配了对比模型")
	}
	primary := agentSettings{provider: "openai", model: "primary-model", budgetAccount: "usd-primary"}
	for _, test := range []struct {
		name, field, value string
		want               bool
	}{
		{"明确启用", "", "", true},                                    // 正常配置固定独立 CNY 账户。
		{"缺 Key", "DEEPSEEK_API_KEY", "", false},                  // 不能等到付费时才发现凭据缺失。
		{"重复账户", "EVAL_COMPARISON_ACCOUNT", "usd-primary", false}, // 禁止币种混账。
		{"相同模型", "EVAL_COMPARISON_MODEL", "primary-model", false}, // 不能冒充跨模型对照。
		{"单次超过总额", "EVAL_COMPARISON_TOTAL_CNY", "0.1", false},     // 不能建立永远无法完成首次预留的安排。
	} {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]string{"EVAL_COMPARISON_ENABLED": "true", "DEEPSEEK_API_KEY": "demo-comparison-key"}
			if test.field != "" {
				values[test.field] = test.value
			}
			value, err := loadComparison(config.NewReader(func(key string) (string, bool) { v, ok := values[key]; return v, ok }), primary)
			if (err == nil) != test.want {
				t.Fatalf("允许=%t err=%v", test.want, err)
			}
			if err == nil && (value.currency != "CNY" || value.budgetAccount != "phase-07-08" || value.model != "deepseek-flash" || value.totalMicros != 10_000_000 || value.comparison != nil) {
				t.Fatal("对比配置没有固定独立账户")
			}
		})
	}
}
