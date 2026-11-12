// 本文件：验证工具实际使用的参数、输入边界和诊断菜单隔离。
//
// 阅读建议：这些测试只检查项目契约，不把候选库的未采纳行为当成新要求。

package tools

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestParameterNumbers 固定 D01–D03 的选择，验证实际读取的是范围内的 int。
func TestParameterNumbers(t *testing.T) {
	validator := schemaGoldenRegistry().tools["query_metrics"].parameters
	for _, testcase := range []struct {
		raw  string // 保留原始数字写法。
		want int    // 程序最终使用的值。
	}{
		{"0", 0}, {"3", 3}, {"3.0", 3}, {"1e3", 1000},
		{"3.0000000000000001", 3}, {"3600.0000000000001", 3600}, {"1e-1000", 0},
	} {
		t.Run(testcase.raw, func(t *testing.T) {
			t.Parallel() // 共用已解析规则，不同调用的默认值和参数树不能互相污染。
			args, err := validator.validate([]byte(`{"query":"up","lookback_seconds":` + testcase.raw + `}`))
			if err != nil {
				t.Fatal(err)
			}
			got, ok := args["lookback_seconds"].(int)
			if !ok || got != testcase.want || args["step_seconds"] != 15 {
				t.Fatalf("整数转换或默认值不符：%+v", args)
			}
		})
	}
}

// TestParameterBoundaries 验证从原始字段构造规则后，零下界、正则和 Unicode 长度仍然有效。
func TestParameterBoundaries(t *testing.T) {
	registry := schemaGoldenRegistry()
	for _, testcase := range []struct {
		name, tool, raw string // 使用实际登记的工具规则。
		bad             bool   // 必须拒绝的输入。
	}{
		{"负回看", "query_metrics", `{"query":"up","lookback_seconds":-1}`, true},
		{"负耗时", "search_traces", `{"service":"orders","min_duration_ms":-1}`, true},
		{"小数", "query_metrics", `{"query":"up","lookback_seconds":3.5}`, true},
		{"数字字符串", "query_metrics", `{"query":"up","lookback_seconds":"3"}`, true},
		{"步长零", "query_metrics", `{"query":"up","step_seconds":0}`, true},
		{"数字溢出", "query_metrics", `{"query":"up","lookback_seconds":1e309}`, true},
		{"重复字段", "query_metrics", `{"query":"up","query":"down"}`, true},
		{"缺必填", "query_metrics", `{}`, true},
		{"多余字段", "query_metrics", `{"query":"up","extra":1}`, true},
		{"合法服务名", "search_traces", `{"service":"order-service"}`, false},
		{"非法服务名", "search_traces", `{"service":"1orders"}`, true},
		{"合法表名", "mysql_show_indexes", `{"table":"order_items"}`, false},
		{"非法表名", "mysql_show_indexes", `{"table":"order-items"}`, true},
		{"合法trace", "get_trace", `{"trace_id":"abcdef0123456789"}`, false},
		{"非法trace", "get_trace", `{"trace_id":"ABCDEF0123456789"}`, true},
		{"未知枚举", "redis_info", `{"section":"outside"}`, true},
		{"非法UTF8", "query_metrics", "{\"query\":\"\xff\"}", true},
		{"输入上限", "query_metrics", `{"query":"up"}` + strings.Repeat(" ", 8192-len(`{"query":"up"}`)), false},
		{"输入超限", "query_metrics", `{"query":"up"}` + strings.Repeat(" ", 8193-len(`{"query":"up"}`)), true},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			_, err := registry.tools[testcase.tool].parameters.validate([]byte(testcase.raw))
			if (err != nil) != testcase.bad {
				t.Fatalf("校验结果不符：%v", err)
			}
		})
	}
	// 规则按 Unicode 字符数计数，不能把中文或 emoji 的 UTF-8 字节数当成长度。
	for _, character := range []string{"中", "😀"} {
		for _, count := range []int{256, 257} {
			raw, _ := json.Marshal(map[string]string{"service": "orders", "keyword": strings.Repeat(character, count)})
			_, err := registry.tools["search_logs"].parameters.validate(raw)
			if (err != nil) != (count > 256) {
				t.Fatalf("Unicode 长度不符：字符=%s 数量=%d error=%v", character, count, err)
			}
		}
	}
}

// TestParameterRules 证明修改诊断窗口后不会从缓存规则补回旧默认值，未变规则可以共用。
func TestParameterRules(t *testing.T) {
	registry := schemaGoldenRegistry()
	base := registry.tools["query_metrics"].parameters
	args, err := base.validate([]byte(`{"query":"up"}`))
	if err != nil || args["lookback_seconds"] != 300 || args["step_seconds"] != 15 {
		t.Fatalf("基础默认值不是 int：%+v %v", args, err)
	}
	diagnosis := registry.ForDiagnosis(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), nil)
	changed := diagnosis.tools["query_metrics"].parameters
	args, err = changed.validate([]byte(`{"query":"up"}`))
	if err != nil {
		t.Fatal(err)
	}
	// 这两个字段必须留给 diagnosisWindow 按故障起点计算，不能由 Schema 填旧默认值。
	if _, exists := args["lookback_seconds"]; exists {
		t.Fatalf("副本仍补了固定窗口：%+v", args)
	}
	if _, exists := args["step_seconds"]; exists {
		t.Fatalf("副本仍补了固定步长：%+v", args)
	}
	if base == changed || diagnosis.tools["service_topology"].parameters != registry.tools["service_topology"].parameters {
		t.Fatal("变更规则没有独立解析，或未变规则被重复解析")
	}
	args, err = base.validate([]byte(`{"query":"up"}`))
	if err != nil || args["lookback_seconds"] != 300 || args["step_seconds"] != 15 {
		t.Fatal("修改诊断规则污染了基础菜单")
	}
}

// TestInvalidParameterDefinition 定义来自代码，坏默认值不能等到第一次工具调用才暴露。
func TestInvalidParameterDefinition(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("构建菜单时放行了超出范围的默认值")
		}
	}()
	Schema{Type: "object", Properties: map[string]Property{"limit": integerProperty("条数", 1, 10, 11)}}.resolve()
}
