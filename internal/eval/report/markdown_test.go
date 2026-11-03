// 本文件：验证第二系统的导出不会继续被标成自建靶场。
//
// 主要内容：TestMarkdownTarget 检查同一个导出器如何保持系统归属和原费用口径。
// TestMarkdownCollectionQuality 检查三种采集状态分表，原诊断和费用仍然可追溯。
// 阅读建议：测试使用空结果；数字只从调用方给出的真实批次对象读取。

package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

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
