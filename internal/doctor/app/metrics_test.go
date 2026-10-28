// 本文件：检查预算停止原因能被监控读到，而且不会随错误文字不断增加标签。
//
// 页面有中文原因，指标则只允许几个固定分类。这里连续切换原因，再恢复可用，
// 确认旧原因会归零；否则仪表盘可能一直显示已经解除的故障。
//
// 主要内容：TestBudgetMetricReasons 检查原因切换、未知分类和恢复。
// 阅读建议：测试只读取独立的内存注册表，不访问数据库或模型。

package app

import (
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
)

// TestBudgetMetricReasons 用真正导出的指标确认标签数量和旧原因清理，而不检查内部字段。
func TestBudgetMetricReasons(t *testing.T) {
	common := telemetry.NewMetrics("doctor")
	metrics, err := newDoctorMetrics(common, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	// 同一份注册表连续经历这些状态，才能发现旧原因没有清零的问题。
	cases := []struct {
		name   string
		value  run.BudgetStatus
		active string
	}{
		{"总额用尽", run.BudgetStatus{Blocked: true, ReasonCode: "total_limit"}, "total_limit"},
		{"单次预留不足取代旧原因", run.BudgetStatus{Blocked: true, ReasonCode: "request_reservation"}, "request_reservation"},
		{"实际费用超出预留", run.BudgetStatus{Blocked: true, ReasonCode: "accounting_guard"}, "accounting_guard"},
		{"任意错误文字不能新增标签", run.BudgetStatus{Blocked: true, ReasonCode: "任意任务编号-123"}, "other"},
		{"恢复可用后全部归零", run.BudgetStatus{}, ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			metrics.budget(test.value)
			// 从公开采集结果断言，不能只检查内存字段而漏掉注册或标签错误。
			families, err := common.Registry.Gather()
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, family := range families {
				// 同一个注册表还包含通用运行时指标，此处只验证预算原因这一组。
				if family.GetName() != "doctor_llm_budget_blocked_reason" {
					continue
				}
				found = true
				if len(family.Metric) != 4 {
					t.Fatalf("原因序列数=%d", len(family.Metric))
				}
				for _, sample := range family.Metric {
					var reason string
					for _, label := range sample.Label {
						if label.GetName() == "reason" {
							reason = label.GetValue()
						}
					}
					want := float64(0)
					if reason == test.active {
						want = 1
					}
					if sample.GetGauge().GetValue() != want {
						t.Fatalf("%s=%v，期望%v", reason, sample.GetGauge().GetValue(), want)
					}
				}
			}
			if !found {
				t.Fatal("没有导出预算原因")
			}
		})
	}
}
