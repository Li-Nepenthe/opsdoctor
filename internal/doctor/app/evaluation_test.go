// 本文件：确认实验组差异由执行菜单保证，不靠模型自觉遵守提示。
//
// 主要内容：TestEvaluationGroups 检查 E0 无工具、E2 才有手册、E3 没有调用链。
// 阅读建议：这里只列菜单和尝试被移除的入口，假后端不会发出任何请求。

package app

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// TestEvaluationGroups 每组都从同一基础菜单创建副本，顺序执行也不能污染下一组。
func TestEvaluationGroups(t *testing.T) {
	index, err := tools.LoadRunbooks(fstest.MapFS{"general.md": &fstest.MapFile{Data: []byte("# 排查\n\n先核对时间，再核对请求。")}})
	if err != nil {
		t.Fatal(err)
	}
	base := tools.New(tools.Dependencies{Traces: &struct{ source.TraceSource }{}}).WithRunbooks(index)
	for _, test := range []struct {
		group         string
		count         int
		traces, books bool
	}{
		{"", 3, true, false},    // 正常医生保持原工具，不自动得到实验手册。
		{"E0", 0, false, false}, // 只有告警与架构输入。
		{"E1", 3, true, false},  // 完整取证，不含手册。
		{"E2", 4, true, true},   // 在 E1 上只增加通用手册。
		{"E3", 1, false, false}, // 只移除两种调用链工具。
	} {
		t.Run(test.group, func(t *testing.T) {
			input := agent.Input{Tools: base}
			start := time.Now().UTC()
			if err := prepareDiagnosis(run.Task{EvaluationGroup: test.group, FaultStartedAt: start}, &input); err != nil {
				t.Fatal(err)
			}
			menu := map[string]bool{}
			for _, definition := range input.Tools.List() {
				menu[definition.Name] = true
			}
			if len(menu) != test.count || menu["get_trace"] != test.traces || menu["runbook_search"] != test.books || !input.FaultStartedAt.Equal(start) {
				t.Fatalf("菜单错误：%v", menu)
			}
			if !test.traces && input.Tools.Run(t.Context(), "get_trace", []byte(`{}`)).Status != "invalid_argument" {
				t.Fatal("隐藏工具仍可执行")
			}
		})
	}
}
