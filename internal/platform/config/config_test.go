package config

import (
	"strings"
	"testing"
	"time"
)

// TestReader 验证缺省、显式空值、格式和边界，避免错误配置带到服务运行以后才暴露。
func TestReader(t *testing.T) {
	// 每个用例独立注入环境，不使用 t.Setenv，所以可以安全地并行测试。
	for _, test := range []struct {
		name    string // name 同时作为用例名和变量名，便于检查错误是否指向正确位置。
		value   string // value 是演示输入，不是真实凭据。
		present bool   // present 区分缺失与显式空值。
		kind    string // kind 选择对应的类型读取器。
		want    any    // want 是期望的有类型值。
		invalid bool   // invalid 表示必须拒绝该配置。
	}{
		{"default", "", false, "int", 20, false},
		{"minimum", "0", true, "int", 0, false},
		{"maximum", "100", true, "int", 100, false},
		{"too_large", "101", true, "int", nil, true},
		{"bad_number", "private-value", true, "int", nil, true},
		{"empty", " ", true, "int", nil, true},
		{"duration", "800ms", true, "duration", 800 * time.Millisecond, false},
		{"no_unit", "800", true, "duration", nil, true},
		{"negative", "-1s", true, "duration", nil, true},
		{"true", "true", true, "bool", true, false},
		{"shorthand", "1", true, "bool", nil, true},
		{"trim", " local ", true, "string", "local", false},
		{"required", "", false, "string", nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := NewReader(func(string) (string, bool) { return test.value, test.present })
			var got any
			var err error
			switch test.kind {
			case "int":
				got, err = reader.Int(test.name, 20, 0, 100)
			case "duration":
				got, err = reader.Duration(test.name, time.Second, time.Millisecond, time.Minute)
			case "bool":
				got, err = reader.Bool(test.name, false)
			case "string":
				got, err = reader.String(test.name, "")
			}
			if test.invalid {
				if err == nil || !strings.Contains(err.Error(), test.name) || strings.Contains(err.Error(), "private-value") {
					t.Fatalf("错误必须定位变量且不泄露原始值：%v", err)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("得到 %v / %v，期望 %v", got, err, test.want)
			}
		})
	}
}
