// 本文件：验证不同诊断的默认时间范围不会串用，实验组不能调用被移除的工具。
//
// 主要内容：TestDiagnosisWindow 检查默认值、显式覆盖和点数预算；TestDiagnosisMenu 检查副本隔离。
// 阅读建议：时钟是固定函数，不用真实等待五分钟，也不依赖计算机运行速度。

package tools

import (
	"context"
	"testing"
	"time"
)

// TestDiagnosisWindow 比较传给后端的实际参数，不只检查公开菜单中的说明。
func TestDiagnosisWindow(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name         string
		elapsed      time.Duration
		args         arguments
		window, step int
	}{
		{"刚开始含五分钟对照", 0, arguments{}, 300, 15},                                                      // 保留故障前的健康样本。
		{"后续调用仍含故障起点", 10 * time.Minute, arguments{}, 900, 15},                                      // 窗口随调查经过的时间增长。
		{"过久不突破后端上限", 2 * time.Hour, arguments{}, 3600, 15},                                         // 一小时仍只有 241 个点。
		{"显式即时查询", time.Minute, arguments{"lookback_seconds": 0}, 0, 15},                            // 0 不是“未提供”。
		{"显式查更早数据", time.Minute, arguments{"lookback_seconds": 1800, "step_seconds": 30}, 1800, 30}, // 模型可以主动扩大范围。
	} {
		t.Run(test.name, func(t *testing.T) {
			next := func(_ context.Context, args arguments) (output, error) {
				if args.integer("lookback_seconds") != test.window || args.integer("step_seconds") != test.step {
					t.Fatalf("实际参数 %v，期望窗口 %d、步长 %d", args, test.window, test.step)
				}
				return output{}, nil
			}
			_, err := diagnosisWindow(next, "query_metrics", start, func() time.Time { return start.Add(test.elapsed) })(t.Context(), test.args)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestDiagnosisMenu 证明 E0 的空菜单不只是展示限制，基础注册表的默认值也不会被改掉。
func TestDiagnosisMenu(t *testing.T) {
	registry := New(Dependencies{Metrics: &unusedSources{}})
	first := registry.ForDiagnosis(time.Now(), map[string]bool{"query_metrics": true})
	second := registry.ForDiagnosis(time.Now(), map[string]bool{})
	if len(first.List()) != 1 || len(second.List()) != 0 || len(registry.List()) != 5 {
		t.Fatal("诊断之间的菜单发生串用")
	}
	if registry.tools["query_metrics"].definition.Parameters.Properties["lookback_seconds"].Default != 300 {
		t.Fatal("修改副本破坏了基础菜单")
	}
	if result := second.Run(t.Context(), "query_metrics", []byte(`{"query":"up"}`)); result.Status != "invalid_argument" {
		t.Fatalf("空菜单仍执行工具：%+v", result)
	}
}
