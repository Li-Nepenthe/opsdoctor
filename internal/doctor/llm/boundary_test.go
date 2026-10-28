// 本文件：验证发往模型的内容边界与整数金额计算。
//
// 使用编造的密钥形状，不读本机环境，也不会连接任何供应商。
// 主要内容：TestRedaction 检查所有入口的字符串形状；TestMoney 检查精度与非法配置。
// 阅读建议：JSON 里还嵌着 JSON 文本的用例，模拟日志正文被工具包起来的情况。

package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRedaction 覆盖密钥、Authorization、JWT、赋值语法和嵌套日志，不依赖模型自觉保密。
func TestRedaction(t *testing.T) {
	redactor := NewRedactor("known-secret-demo-value")
	for _, testcase := range []struct{ name, text, secret string }{
		{"密钥", "key sk-demo123456789", "sk-demo123456789"},                          // 普通文字里的 Key。
		{"请求头", "Authorization: Bearer demo.token.value", "demo.token.value"},       // Bearer 令牌。
		{"JWT", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.signature", "eyJhbGci"},       // 无前缀 JWT。
		{"密码赋值", "password=demo-password", "demo-password"},                         // 常见配置日志。
		{"已知秘密", "message known-secret-demo-value", "known-secret-demo-value"},      // 形状不特殊的本机 Key。
		{"JSON字段", `{"password":"demo-password"}`, "demo-password"},                 // 结构化日志。
		{"嵌套正文", `{"content":"{\"password\":\"demo-password\"}"}`, "demo-password"}, // JSON 套在字符串里。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			clean := redactor.Text(testcase.text)
			if strings.Contains(clean, testcase.secret) || !strings.Contains(clean, "REDACTED") {
				t.Fatal("秘密未清理")
			}
			if redactor.Text(clean) != clean {
				t.Fatal("重复脱敏改变了结果")
			}
		})
	}
	raw, _ := redactor.JSON([]byte(`{"value":9007199254740993,"text":"safe"}`))
	if !json.Valid(raw) || !strings.Contains(string(raw), "9007199254740993") {
		t.Fatal("改变了大整数或 JSON 结构")
	}
}

// TestMoney 不接受模糊的计价格式，并且很小的费用也向上记到账本里。
func TestMoney(t *testing.T) {
	for text, want := range map[string]int64{"20": 20000000, "2.5": 2500000, "0.000001": 1, "0": 0} {
		value, err := Money(text)
		if err != nil || value != want {
			t.Fatalf("%s => %d %v", text, value, err)
		}
	}
	for _, text := range []string{"-1", "1e3", "NaN", "1.0000001", "", "1000001"} {
		// 这些格式会让费用比较失去明确含义，必须在启动前拒绝。
		if _, err := Money(text); err == nil {
			t.Fatalf("放行 %q", text)
		}
	}
	if got := (Prices{InputMicros: 2000000, OutputMicros: 8000000}).Cost(3, 2); got != 22 {
		t.Fatalf("cost=%d", got)
	}
}
