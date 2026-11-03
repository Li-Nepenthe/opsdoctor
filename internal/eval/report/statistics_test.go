// 本文件：证明统计排除不会把已经发生的费用一起删掉。
//
// 主要内容：TestStatistics 验证有效样本、缺失评分和失败费用的分开计算。
// TestStatisticsCollectionQuality 验证阶段 9 的异常采集单列、阶段 8 的原口径保持。
// 阅读建议：一份有效报告加两份被排除记录，质量统计 n 应为一，费用仍是三份。

package report

import (
	"bytes"
	"encoding/json"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"strings"
	"testing"
)

// TestStatistics 不用混合模型或可靠性失败稀释质量分母，也不隐藏它们花的钱。
func TestStatistics(t *testing.T) {
	rows := []store.EvalResult{}
	for _, value := range []Summary{
		{Model: "model-a", ComponentMatch: true, CostMicros: 100000, Currency: "USD", DurationKnown: true, Seconds: 12}, // 唯一有效样本。
		{Model: "model-a", ReliabilityFailure: true, CostMicros: 200000, Currency: "USD"},                               // 未收到模型响应。
		{Model: "model-a", MixedModel: true, CostMicros: 300000, Currency: "USD"},                                       // 中途换模型。
	} {
		raw, _ := json.Marshal(value)
		rows = append(rows, store.EvalResult{Scenario: "S01", Group: "E1", Data: raw})
	}
	// 格式错误的评分没有可用分数，不能被当作一次机制零分。
	rows[0].Judgement = json.RawMessage(`{"status":"failed","cost_micros":1000}`)
	var output bytes.Buffer
	if err := Statistics(&output, rows); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"1.000 [1.000, 1.000] n=1", "12.000 [12.000, 12.000] n=1", "USD 0.600000", "CNY 0.001000", "1 / 1 / 0", "未测量"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("遗漏 %s：%s", want, output.String())
		}
	}
}

// TestStatisticsCollectionQuality 让受影响和未知样本退出质量分母，但保留全部费用。
func TestStatisticsCollectionQuality(t *testing.T) {
	rows := []store.EvalResult{}
	for index, quality := range []string{QualityNormal, QualityAffected, QualityUnknown, QualityNormal, QualityNormal} {
		value := Summary{DataQuality: quality, CollectorAudit: collectorAuditFixture(), Model: "gpt-6.1-terra", ComponentMatch: index == 0, CostMicros: int64(index+1) * 100000, Currency: "USD", Rounds: 2, Tools: 3, DurationKnown: true, Seconds: 12}
		if index == 3 {
			value.CollectorAudit = nil // 写了 normal 也不能替代真实的窗口审计。
		}
		if index == 4 {
			value.CollectorAudit.Status = "failed"
		}
		raw, _ := json.Marshal(value)
		rows = append(rows, store.EvalResult{RunID: "phase09-test", Scenario: "OT01", Group: "E1", Data: raw, Judgement: json.RawMessage(`{"status":"ok","cost_micros":1000,"coverage":0.8,"grade":{"mechanism":"correct"}}`)})
	}
	var output bytes.Buffer
	if err := Statistics(&output, rows); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"1.000 [1.000, 1.000] n=1", "0.800 [0.800, 0.800] n=1", "12.000 [12.000, 12.000] n=1", "USD 0.100 [0.100, 0.100] n=1", "USD 1.500000", "CNY 0.005000", "采集受影响 / 采集未知", "| 1 / 3 |"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("遗漏 %s：%s", want, output.String())
		}
	}
	// 相同原记录放回阶段 8 时不追溯增加审计要求，历史分母与导出格式都保持。
	for index := range rows {
		rows[index].RunID, rows[index].Scenario = "phase08-test", "S01"
	}
	output.Reset()
	if err := Statistics(&output, rows); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "0.200 [0.000, 1.000] n=5") || strings.Contains(output.String(), "采集受影响 / 采集未知") {
		t.Fatal("阶段 9 采集门槛改变了阶段 8 历史统计", output.String())
	}
}
