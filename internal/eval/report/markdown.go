// 本文件：将数据库中的评测结果排成便于审阅的 Markdown 表格。
//
// 这里只展示已经发生的运行。空槽位明确写“未完成”，失败不会被跳过。单次组件
// 一致不能代表整套系统的准确率，正式统计还要结合机制、证据覆盖和样本数量。
//
// 主要内容：Markdown 输出批次及结果行；cell 防止报告文字破坏表格结构。
// 阅读建议：金额始终附币种；没有真实扣费接口时，只能称作用量账本费用。

package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
)

// Markdown 不在导出时重新评分，相同数据库快照应当生成相同的表格。
func Markdown(output io.Writer, run store.EvalRun, rows []store.EvalResult) error {
	if _, err := fmt.Fprintf(output, "# 评测试跑 %s\n\n这张表来自自建靶场的少量试跑，不是正式评测结论。机制尚未由评委评分。OpenAI 金额是用量账本，实际扣费由用户在平台核对。\n\n批次 token：%d / %d（含未确认预留）。\n\n| 场景 | 轮次 | 组 | 诊断 | 状态 | 组件一致 | 轮数 | 工具 | 输入 / 输出 token | 账本费用 | 耗时秒 | 提交方式 | 混合模型 |\n|---|---:|---|---|---|---|---:|---:|---:|---:|---:|---|---|\n", cell(run.ID), run.TokensUsed, run.TokenLimit); err != nil {
		return err
	}
	for _, row := range rows {
		// 中断留下的空槽位也要显示，不能在导出时悄悄少算一次失败。
		var value Summary
		if len(row.Data) == 0 {
			if _, err := fmt.Fprintf(output, "| %s | %d | %s | %s | 未完成 | | | | | | | | |\n", cell(row.Scenario), row.Iteration, cell(row.Group), cell(row.DiagnosisID)); err != nil {
				return err
			}
			continue
		}
		if err := json.Unmarshal(row.Data, &value); err != nil {
			return fmt.Errorf("保存的结果无法解码：%w", err)
		}
		// 墙钟可能被校时调回去。保留原值并标成无效，不能当作负耗时参与平均。
		elapsed := fmt.Sprintf("%.3f", value.Seconds)
		if value.Seconds < 0 {
			elapsed = fmt.Sprintf("无效（原值 %.3f）", value.Seconds)
		} else if !value.DurationKnown {
			elapsed = fmt.Sprintf("未测量（历史字段 %.3f）", value.Seconds)
		}
		tokens := fmt.Sprintf("%d / %d", value.InputTokens, value.OutputTokens)
		if value.InputTokens == 0 && value.OutputTokens == 0 && value.CostMicros > 0 {
			tokens = "未知（原记 0 / 0）" // 没收到用量的请求仍占预留额，不代表供应商免费处理。
		}
		mode := value.ReportMode
		if len(value.Report) == 0 || string(value.Report) == "null" {
			mode = "无报告" // 历史任务的默认 voluntary 标志不能说明失败时真的提交了报告。
		}
		if _, err := fmt.Fprintf(output, "| %s | %d | %s | %s | %s | %t | %d | %d | %s | %s %.6f | %s | %s | %t |\n", cell(row.Scenario), row.Iteration, cell(row.Group), cell(row.DiagnosisID), cell(value.Status), value.ComponentMatch, value.Rounds, value.Tools, tokens, cell(value.Currency), float64(value.CostMicros)/1e6, elapsed, cell(mode), value.MixedModel); err != nil {
			return err
		}
	}
	return nil
}

// cell 只整理表格分隔符，不改报告的数据库原文。
func cell(text string) string {
	return strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ").Replace(text)
}
