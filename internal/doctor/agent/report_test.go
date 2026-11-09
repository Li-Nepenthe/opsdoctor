// 本文件：验证报告双解码保留旧输出，同时修复整数写法与真实步骤引用的判断。
//
// 主要内容：TestReportOutputBytes 比较替换前基线；TestReportNumberReferences 覆盖 D04/D05；
// TestReportInputLimits 检查输入保证；TestReportSchemaCache 检查并发复用与组件隔离。
// 阅读建议：用于校验的 float64 副本不能成为输出来源，字节基线由旧实现生成。

package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
)

// TestReportOutputBytes 固定旧实现的实际输出，覆盖 D06、D07、D09 和多种数字文字。
func TestReportOutputBytes(t *testing.T) {
	raw, err := os.ReadFile("testdata/report_output.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name   string // 基线中的输入类别。
		Input  string // 当时传给旧 validateReport 的原始 JSON。
		Output string // 旧函数返回的完整字节，不能重新格式化。
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	// 只比较保存好的基线，不在测试时用新函数生成预期值。
	for _, testcase := range cases {
		t.Run(testcase.Name, func(t *testing.T) {
			got, err := validateReport([]byte(testcase.Input), []string{"unknown"}, map[int]bool{1: true})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, []byte(testcase.Output)) {
				t.Fatalf("报告字节改变：\n当前 %s\n旧输出 %s", got, testcase.Output)
			}
		})
	}
}

// TestReportNumberReferences 先按已验证的 float64 读取步骤，输出仍保留原数字写法。
func TestReportNumberReferences(t *testing.T) {
	for _, number := range []string{"1.0", "1e0", "3.0000000000000001"} {
		t.Run(number, func(t *testing.T) {
			raw := strings.Replace(reportArgs(1), `"step":1`, `"step":`+number, 1)
			got, err := validateReport([]byte(raw), []string{"unknown"}, map[int]bool{1: true, 3: true})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(got, []byte(`"step":`+number)) {
				t.Fatalf("校验步骤时改写了输出：%s", got)
			}
		})
	}
	// 规范化写法不等于可以编造步骤；实际编号仍必须在本次记录里。
	raw := strings.Replace(reportArgs(1), `"step":1`, `"step":2.0`, 1)
	if _, err := validateReport([]byte(raw), []string{"unknown"}, map[int]bool{1: true}); err == nil {
		t.Fatal("放行了不存在的步骤")
	}
}

// TestReportInputLimits 固定大小、深度、重复键、溢出数字和原来的 UTF-8 策略。
func TestReportInputLimits(t *testing.T) {
	base := reportArgs(1)
	for _, testcase := range []struct {
		name, raw string // 保留非法输入的原始字节。
		bad       bool   // 必须拒绝的输入。
	}{
		{"重复顶层字段", strings.Replace(base, `"summary":"测试结论"`, `"summary":"first","summary":"second"`, 1), true},
		{"重复嵌套字段", strings.Replace(base, `"args":{}`, `"args":{"x":1,"x":2}`, 1), true},
		{"多个值", base + `{}`, true},
		{"深度超限", strings.Replace(base, `"args":{}`, `"args":{"x":`+strings.Repeat("[", 20)+"0"+strings.Repeat("]", 20)+`}`, 1), true},
		{"大小边界", base + strings.Repeat(" ", (16<<10)-len(base)), false},
		{"大小超限", base + strings.Repeat(" ", (16<<10)+1-len(base)), true},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			_, err := validateReport([]byte(testcase.raw), []string{"unknown"}, map[int]bool{1: true})
			if (err != nil) != testcase.bad {
				t.Fatalf("输入边界不符：%v", err)
			}
		})
	}
	// D08 是审核接受的新增拒绝；提示固定为中文，不暴露解码器内部类型。
	large := strings.Replace(base, `"args":{}`, `"args":{"number":1e309}`, 1)
	if _, err := validateReport([]byte(large), []string{"unknown"}, map[int]bool{1: true}); err == nil || err.Error() != "报告里的数字超出范围" {
		t.Fatalf("溢出数字提示不符：%v", err)
	}
	invalidUTF8 := strings.Replace(base, `"summary":"测试结论"`, "\"summary\":\"\xff\"", 1)
	got, err := validateReport([]byte(invalidUTF8), []string{"unknown"}, map[int]bool{1: true})
	if err != nil || !utf8.Valid(got) || !bytes.Contains(got, []byte("\ufffd")) {
		t.Fatalf("报告原有 UTF-8 替换行为改变：%s %v", got, err)
	}
}

// TestReportSchemaLimits 用真实报告规则检查 Unicode 字符数和证据数量。
func TestReportSchemaLimits(t *testing.T) {
	for _, character := range []string{"中", "😀"} {
		for _, count := range []int{1000, 1001} {
			raw := strings.Replace(reportArgs(1), "测试结论", strings.Repeat(character, count), 1)
			_, err := validateReport([]byte(raw), []string{"unknown"}, map[int]bool{1: true})
			if (err != nil) != (count > 1000) {
				t.Fatalf("报告字符长度判断不符：字符=%s 数量=%d error=%v", character, count, err)
			}
		}
	}
	// 数组里每条证据都合法，失败只能来自总数量，而不是字段遗漏或步骤不存在。
	for _, count := range []int{40, 41} {
		var report map[string]any
		if err := json.Unmarshal([]byte(reportArgs(1)), &report); err != nil {
			t.Fatal(err)
		}
		items := make([]any, count)
		for index := range items {
			items[index] = map[string]any{"step": 1, "observation": "x"}
		}
		report["evidence"] = items
		raw, _ := json.Marshal(report)
		_, err := validateReport(raw, []string{"unknown"}, map[int]bool{1: true})
		if (err != nil) != (count > 40) {
			t.Fatalf("报告证据数量判断不符：数量=%d error=%v", count, err)
		}
	}
}

// TestReportSchemaCache 验证并发首次解析复用同一结果，不同组件表不会串用枚举。
func TestReportSchemaCache(t *testing.T) {
	components := []string{"unknown", "cache_first"}
	rules := make([]*jsonschema.Resolved, 12)
	errors := make([]error, len(rules))
	var workers sync.WaitGroup
	start := make(chan struct{})
	// 所有调用同时开始，覆盖首次初始化而不只是读取已经准备好的缓存。
	for index := range rules {
		workers.Go(func() {
			<-start
			rules[index], errors[index] = resolvedReportSchema(components)
		})
	}
	close(start)
	workers.Wait()
	for index, rule := range rules {
		if errors[index] != nil || rule == nil || rule != rules[0] {
			t.Fatalf("并发解析没有复用同一规则：%d %v", index, errors[index])
		}
	}
	raw := strings.Replace(reportArgs(1), `"component":"unknown"`, `"component":"cache_first"`, 1)
	if _, err := validateReport([]byte(raw), components, map[int]bool{1: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := validateReport([]byte(raw), []string{"unknown", "cache_second"}, map[int]bool{1: true}); err == nil {
		t.Fatal("缓存串用了另一个组件表")
	}
}
