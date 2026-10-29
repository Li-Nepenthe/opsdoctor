// 本文件：用固定记录验证组件精确比对、模型变更排除和失败保留。
//
// 主要内容：TestSummarize 比较实际存下来的报告；TestMarkdown 保留未完成行。
// TestMarkdownUnknown 明确展示未知用量和无效时间，不让失败看起来像正常提交。
// TestMissingDuration 检查没有单调时钟测量的老记录不会被补算成有效耗时。
// 阅读建议：组件别名和大小写变化都不自动纠正，避免把宽松匹配冒充精确比较。

package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
)

// TestSummarize 覆盖正常报告、混合模型、只有新模型步骤、拼错组件和没有报告。
func TestSummarize(t *testing.T) {
	for _, test := range []struct {
		name, component string
		models          []string
		mixed, match    bool
	}{
		{"正常一致", "inventory", []string{"mock-a", "mock-a"}, false, true},
		{"两种模型", "inventory", []string{"mock-a", "mock-b"}, true, true},
		{"恢复后只有新模型", "inventory", []string{"mock-b"}, true, true},
		{"大小写不同", "Inventory", []string{"mock-a"}, false, false},
		{"没有报告", "", nil, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			start := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
			end := start.Add(-time.Second) // 故意模拟校时倒退，耗时必须仍采用独立测量值。
			duration := int64(12000)
			detail := store.Detail{Diagnosis: store.Diagnosis{ID: "test", Model: "mock-a", Status: "failed", Currency: "USD"}, StartedAt: &start, FinishedAt: &end, DurationMS: &duration, Error: "保留失败", CostMicros: 123}
			// 没报告的用例保留空值；不能替失败诊断填入标准答案。
			if test.component != "" {
				detail.Report, _ = json.Marshal(map[string]any{"root_cause": map[string]string{"component": test.component}})
			}
			for _, model := range test.models {
				detail.Steps = append(detail.Steps, agent.Step{Kind: "llm", Model: model})
			}
			got := Summarize(detail, "inventory")
			if got.MixedModel != test.mixed || got.ComponentMatch != test.match || got.Seconds != 12 || !got.DurationKnown || got.Error != "保留失败" || got.CostMicros != 123 {
				t.Fatalf("summary=%+v", got)
			}
		})
	}
}

// TestMissingDuration 不用老记录的起止时间推算未实际测量过的耗时。
func TestMissingDuration(t *testing.T) {
	start := time.Now()
	end := start.Add(time.Minute)
	got := Summarize(store.Detail{StartedAt: &start, FinishedAt: &end}, "unknown")
	if got.DurationKnown || got.Seconds != 0 {
		t.Fatal("把旧时间戳冒充单调时钟测量")
	}
}

// TestMarkdown 未完成和失败各占一行；币种跟着费用显示，不能汇成无单位的总数。
func TestMarkdown(t *testing.T) {
	// 第一行故意没有结果，第二行保留失败，不过滤任何一行。
	raw, _ := json.Marshal(Summary{Status: "failed", Error: "失败", Currency: "USD", CostMicros: 123})
	var output bytes.Buffer
	err := Markdown(&output, store.EvalRun{ID: "test"}, []store.EvalResult{{Scenario: "S01", Group: "E1"}, {Scenario: "S02", Group: "E1", Data: raw}})
	if err != nil || !strings.Contains(output.String(), "未完成") || !strings.Contains(output.String(), "failed") || !strings.Contains(output.String(), "USD 0.000123") {
		t.Fatalf("output=%s err=%v", output.String(), err)
	}
}

// TestMarkdownUnknown 不把未收到账单的零用量、倒序时间和默认提交标志包装成正常测量。
func TestMarkdownUnknown(t *testing.T) {
	raw, _ := json.Marshal(Summary{Status: "failed", Currency: "USD", CostMicros: 53598, Seconds: -1.999, ReportMode: "voluntary"})
	var output bytes.Buffer
	if err := Markdown(&output, store.EvalRun{ID: "test"}, []store.EvalResult{{Scenario: "S10", Group: "E1", Data: raw}}); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"无效（原值 -1.999）", "未知（原记 0 / 0）", "无报告", "USD 0.053598"} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("遗漏异常说明 %s", text)
		}
	}
}
