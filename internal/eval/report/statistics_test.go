// 本文件：证明统计排除不会把已经发生的费用一起删掉。
//
// 主要内容：TestStatistics 验证有效样本、缺失评分和失败费用的分开计算。
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
