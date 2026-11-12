// 本文件：核对第二系统的归属、采集质量分表和完整报告的输出字节。样本里的数字全部取自传入的批次和槽位。

package report

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
)

// TestMarkdownTarget 外部系统明确显示其名字，同时保留美元用量账本的说明。
func TestMarkdownTarget(t *testing.T) {
	var output bytes.Buffer
	if err := Markdown(&output, store.EvalRun{ID: "phase09-test", Target: "otel-demo", TokenLimit: 100}, nil); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "接入系统 otel-demo") || strings.Contains(text, "来自自建靶场") || !strings.Contains(text, "按项目用量账本计") {
		t.Fatal("导出丢失系统归属或费用口径")
	}
}

// TestMarkdownCollectionQuality 不让 affected 或缺少审计的样本出现在正常结果表里。
func TestMarkdownCollectionQuality(t *testing.T) {
	rows := []store.EvalResult{}
	for index, quality := range []string{QualityNormal, QualityAffected, QualityUnknown} {
		value := Summary{DataQuality: quality, CollectorAudit: collectorAuditFixture(), Model: "gpt-6.1-terra", Status: "succeeded", ComponentMatch: true, Currency: "USD", CostMicros: int64(index+1) * 100000, Report: json.RawMessage(`{"root_cause":{"component":"ad"}}`)}
		if quality == QualityAffected {
			value.CollectorAudit.DroppingDataEvents = 1
			value.CollectorAudit.LogDroppedItems = 10
			value.CollectorAudit.MapperErrorLines, value.CollectorAudit.MapperErrorOccurrences = 1, 2
		}
		if quality == QualityUnknown {
			value.DataQuality, value.CollectorAudit = QualityNormal, nil // 缺审计不能被 normal 字段绕过。
		}
		raw, _ := json.Marshal(value)
		rows = append(rows, store.EvalResult{Key: "slot-" + quality, DiagnosisID: "diagnosis-" + quality, Scenario: "OT01", Iteration: 1, Group: "E1", Data: raw})
		if quality == QualityAffected {
			rows[index].Judgement = json.RawMessage(`{"status":"ok","cost_micros":1000,"coverage":0.2,"grade":{"mechanism":"incorrect","reason":"保留原评委理由"}}`)
		}
	}
	var output bytes.Buffer
	if err := Markdown(&output, store.EvalRun{ID: "phase09-test", Target: "otel-demo"}, rows); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	normal := strings.Split(strings.Split(text, "## 采集正常样本")[1], "## 采集受影响样本")[0]
	affected := strings.Split(strings.Split(text, "## 采集受影响样本（不计入正常统计）")[1], "## 采集质量未知样本")[0]
	unknown := strings.Split(strings.Split(text, "## 采集质量未知样本（不计入正常统计）")[1], "## 采集窗口审计")[0]
	if !strings.Contains(normal, "diagnosis-normal") || strings.Contains(normal, "diagnosis-affected") || strings.Contains(normal, "diagnosis-unknown") || !strings.Contains(affected, "diagnosis-affected") || !strings.Contains(unknown, "diagnosis-unknown") {
		t.Fatal("采集分类没有分表隔离", text)
	}
	for _, want := range []string{"USD 0.100000", "USD 0.200000", "USD 0.300000", "USD 0.600000", "CNY 0.001000", "slot-collector.log", strings.Repeat("a", 64), "mapper 行 / 次数", "1 / 2", "无审计", "采集 affected；仅保留原评分", "保留原评委理由", "采集 unknown；不计入正常统计"} {
		if !strings.Contains(text, want) {
			t.Fatalf("导出遗漏 %s：%s", want, text)
		}
	}
	// 阶段 8 不追溯要求这次新增的采集证据，旧表格与原质量分母保持。
	legacy := rows[1]
	legacy.Key, legacy.Scenario = "phase08-old/slot", "S01"
	output.Reset()
	if err := Markdown(&output, store.EvalRun{ID: "phase08-old", Target: "shop"}, []store.EvalResult{legacy}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "采集正常样本") || strings.Contains(output.String(), "采集受影响 / 采集未知") || !strings.Contains(output.String(), "1.000 [1.000, 1.000] n=1") || !strings.Contains(output.String(), "USD 0.200000") {
		t.Fatal("阶段 8 的历史导出口径被改变", output.String())
	}
}

// TestReportOutputGolden 从固定诊断生成摘要，再将成功和失败样本送进实际报表导出器。
func TestReportOutputGolden(t *testing.T) {
	duration := int64(1250)
	detail := store.Detail{
		Diagnosis: store.Diagnosis{ID: "demo-1", Model: "mock-a", Status: "succeeded", Currency: "USD"},
		Steps:     []agent.Step{{Kind: "llm", Model: "mock-a"}}, Report: json.RawMessage(`{"root_cause":{"component":"stock"}}`),
		DurationMS: &duration, TokensIn: 32, TokensOut: 14, Rounds: 2, ToolCalls: 1, CostMicros: 1200, ReportMode: "voluntary",
	}
	raw, err := json.Marshal(Summarize(detail, "stock"))
	if err != nil {
		t.Fatal(err)
	}
	// 同层再加一条未定位的报告。它的评委失败应保留费用，但不能稀释机制分数。
	rows := []store.EvalResult{
		{Key: "a", Scenario: "sample", Iteration: 1, Group: "E1", DiagnosisID: detail.ID, Data: raw,
			Judgement: json.RawMessage(`{"status":"ok","coverage":0.5,"cost_micros":200,"grade":{"mechanism":"partial","reason":"取得部分证据"}}`)},
		{Key: "b", Scenario: "sample", Iteration: 2, Group: "E1", DiagnosisID: "demo-2",
			Data:      json.RawMessage(`{"status":"failed","model":"mock-a","currency":"USD","cost_micros":400,"error":"没有报告"}`),
			Judgement: json.RawMessage(`{"status":"failed","cost_micros":100,"error":"评分不可用"}`)},
	}
	var output bytes.Buffer
	if err := Markdown(&output, store.EvalRun{ID: "demo", Target: "shop", TokensUsed: 46, TokenLimit: 1000}, rows); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		golden string
		got    []byte
	}{
		{"testdata/summary.golden.json", raw},
		{"testdata/report.golden.md", output.Bytes()},
	} {
		want, err := os.ReadFile(item.golden)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(item.got, want) {
			t.Errorf("%s 与实际评测输出不一致：\n%s", item.golden, item.got)
		}
	}
}
