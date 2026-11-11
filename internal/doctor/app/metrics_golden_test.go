// 本文件：通过诊断状态、工具、模型用量和预算的实际记录入口，固定医生对外导出的指标。

package app

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
)

// TestDoctorMetricFamiliesGolden 让每种仪表都产生样本，便于发现改名、漏注册或标签被互换。
func TestDoctorMetricFamiliesGolden(t *testing.T) {
	common := telemetry.NewMetrics("doctor")
	metrics, err := newDoctorMetrics(common, map[string]bool{"query_metrics": true})
	if err != nil {
		t.Fatal(err)
	}
	metrics.changed("shop", "completed", time.Minute)
	metrics.step(agent.Step{Kind: "llm", Model: "mock", Currency: "USD", Usage: llm.Usage{Input: 10, Output: 2, CacheHit: 3}, CostMicros: 5000})
	metrics.step(agent.Step{Kind: "tool", ToolName: "query_metrics", Status: "ok", DurationMS: 250})
	metrics.budget(run.BudgetStatus{Currency: "USD", SpentMicros: 2000, LimitMicros: 10000, Blocked: true, ReasonCode: "request_reservation"})

	// 仅读取 doctor 注册的系列。每个样本都记下标签和值，以区分方向、原因和币种。
	families, err := common.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var got strings.Builder
	for _, family := range families {
		if !strings.HasPrefix(family.GetName(), "doctor_") {
			continue
		}
		fmt.Fprintf(&got, "%s\t%s\t%s\n", family.GetName(), family.GetType(), family.GetHelp())
		for _, sample := range family.GetMetric() {
			var labels []string
			for _, label := range sample.GetLabel() {
				labels = append(labels, fmt.Sprintf("%s=%q", label.GetName(), label.GetValue()))
			}
			fmt.Fprintf(&got, "  %s\n", strings.Join(labels, ","))
		}
	}
	want, err := os.ReadFile("testdata/doctor_metrics.golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != string(want) {
		t.Fatalf("医生指标与基线不同，实际为\n%s", got.String())
	}
}
