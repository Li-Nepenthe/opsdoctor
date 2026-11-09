// 本文件：验证严格解码只检查 JSON 输入边界，不改变数字原文或字段内容。
//
// 主要内容：TestDecodeNumbers 检查数字保真；TestDecodeBoundaries 覆盖重复键、格式和深度。
// 阅读建议：JSON Schema 的类型与范围不在这里测试，它们由调用方交给库。

package jsonstrict

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestDecodeNumbers 同时覆盖浮点数不能精确表示的值和不能表示的数量级。
func TestDecodeNumbers(t *testing.T) {
	for _, raw := range []string{"9007199254740993", "1e-1000", "1e309", "1.0000000000000001", "-0.0"} {
		value, err := Decode([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		number, ok := value.(json.Number)
		if !ok || number.String() != raw {
			t.Fatalf("数字被改写：输入 %s，得到 %T %v", raw, value, value)
		}
	}
}

// TestDecodeBoundaries 保留旧报告的十六层计数，也检查转义后实际相同的重复键。
func TestDecodeBoundaries(t *testing.T) {
	for _, testcase := range []struct {
		name, raw string // 固定输入，不读取外部数据。
		bad       bool   // 是否必须在 Schema 校验前拒绝。
	}{
		{"对象", `{"a":[1,true,null,"中"]}`, false},
		{"顶层重复", `{"a":1,"a":2}`, true},
		{"转义重复", `{"a":1,"\u0061":2}`, true},
		{"嵌套重复", `[{"x":{"a":1,"a":2}}]`, true},
		{"多个值", `{} {}`, true},
		{"尾随坏字符", `{} x`, true},
		{"空输入", ``, true},
		{"未闭合", `{"a":[1}`, true},
		{"孤立结束符", `}`, true},
		{"十六层", strings.Repeat("[", 16) + "0" + strings.Repeat("]", 16), false},
		{"十七层", strings.Repeat("[", 17) + "0" + strings.Repeat("]", 17), true},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			// 大小和 UTF-8 由各入口决定，此处只观察重复键、完整性和递归深度。
			_, err := Decode([]byte(testcase.raw))
			if (err != nil) != testcase.bad {
				t.Fatalf("边界判断不符：%v", err)
			}
		})
	}
}
