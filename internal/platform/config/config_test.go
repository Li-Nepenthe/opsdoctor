// 本文件：用内存环境验证配置读取和错误提示。
//
// TestReader 把缺值、空值、范围和单位分别列成用例；它关心错误能否帮助定位变量，也检查敏感输入没有被回显。
// 读每个用例旁边的理由，再对照 config.go 的分支。

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
		// 环境变量完全没设置时，采用默认整数 20。
		{"default", "", false, "int", 20, false},
		// 下界 0 属于闭区间，应当接受。
		{"minimum", "0", true, "int", 0, false},
		// 上界 100 也属于闭区间，应当接受。
		{"maximum", "100", true, "int", 100, false},
		// 比上界大 1 就必须拒绝，不能截断成 100。
		{"too_large", "101", true, "int", nil, true},
		// 非数字要报错，且不能把可能含秘密的原始值写进错误。
		{"bad_number", "private-value", true, "int", nil, true},
		// 显式设置为空白表示配置有误，不能悄悄退回默认值。
		{"empty", " ", true, "int", nil, true},
		// 带 ms 单位的输入应转为正确的 Duration。
		{"duration", "800ms", true, "duration", 800 * time.Millisecond, false},
		// 不带单位的数字含义不清，必须拒绝。
		{"no_unit", "800", true, "duration", nil, true},
		// 格式合法但低于时长下界，同样不能通过。
		{"negative", "-1s", true, "duration", nil, true},
		// 布尔值使用完整的 true 字样，应正常读取。
		{"true", "true", true, "bool", true, false},
		// 即使其他解析器允许 1 表示真，本项目也要求可读的 true/false。
		{"shorthand", "1", true, "bool", nil, true},
		// 字符串首尾的误输空格会被清理，内容本身保留。
		{"trim", " local ", true, "string", "local", false},
		// 没有提供环境值，也没有有效默认值，必须提示必填项缺失。
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
				// 对失败用例同时检查“拒绝输入”“指明变量”和“不泄露输入”，不能只看 err 非 nil。
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
