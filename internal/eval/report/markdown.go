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
	if _, err := fmt.Fprintf(output, "# 评测记录 %s\n\n这张表来自自建靶场。请按批次配置区分试跑和正式评测；每个场景样本很少，不能代表生产准确率。OpenAI 金额是用量账本，实际扣费由用户在平台核对。\n\n批次 token：%d / %d（含未确认预留）。\n\n| 场景 | 轮次 | 组 | 诊断 | 状态 | 组件一致 | 轮数 | 工具 | 输入 / 输出 token | 账本费用 | 耗时秒 | 提交方式 | 混合模型 |\n|---|---:|---|---|---|---|---:|---:|---:|---:|---:|---|---|\n", cell(run.ID), run.TokensUsed, run.TokenLimit); err != nil {
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
		state := value.Status
		if value.ReliabilityFailure {
			state += "（可靠性；不计入诊断正确率）"
		}
		if row.Attempt == 1 {
			state += "（一次补跑）" // 原记录仍有自己的行，不能只导出这次较好的结果。
		}
		if len(value.Report) == 0 || string(value.Report) == "null" {
			mode = "无报告" // 历史任务的默认 voluntary 标志不能说明失败时真的提交了报告。
		}
		if _, err := fmt.Fprintf(output, "| %s | %d | %s | %s | %s | %t | %d | %d | %s | %s %.6f | %s | %s | %t |\n", cell(row.Scenario), row.Iteration, cell(row.Group), cell(row.DiagnosisID), cell(state), value.ComponentMatch, value.Rounds, value.Tools, tokens, cell(value.Currency), float64(value.CostMicros)/1e6, elapsed, cell(mode), value.MixedModel); err != nil {
			return err
		}
	}
	if err := Statistics(output, rows); err != nil {
		return err
	}
	return gradingRows(output, rows)
}

// cell 只整理表格分隔符，不改报告的数据库原文。
func cell(text string) string {
	return strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ").Replace(text)
}

// gradingRows 保留每份机制理由、评分失败和错误说明，人工校准之后再补“评分有误”的分析。
func gradingRows(output io.Writer, rows []store.EvalResult) error {
	// 显示原理由而不自动推定失败原因，人工校准可据完整步骤重新核对。
	fmt.Fprintln(output, "\n## 评分与失败明细\n\n以下是原始结论与评委理由；评委也是模型，人工一致率未填写前不能视为标准答案。格式、预算、可靠性和评分失败分别显示。推理错误、工具不足、数据缺失以及评分有误的最终分类，需要结合原步骤逐条核对。\n\n| 槽位 | 医生错误 / 预算原因 | 机制 / 评分状态 | 评分理由 | 证据覆盖 | 评委账本 | S12 实际暴露 | 写提议 / 执行 |\n|---|---|---|---|---|---|---|---|")
	for _, row := range rows {
		var result Summary
		if len(row.Data) > 0 && json.Unmarshal(row.Data, &result) != nil {
			return fmt.Errorf("明细结果损坏")
		}
		var grade judgementView
		if len(row.Judgement) > 0 && json.Unmarshal(row.Judgement, &grade) != nil {
			return fmt.Errorf("明细评分损坏")
		}
		label, reason, coverage := "未评分", "", "—"
		if grade.Status == "ok" {
			label, reason, coverage = grade.Grade.Mechanism, grade.Grade.Reason, fmt.Sprintf("%.3f", grade.Coverage)
		} else if grade.Status == "failed" {
			label, reason = "评分失败", grade.Error
		}
		if _, err := fmt.Fprintf(output, "| %s | %s | %s | %s | %s | CNY %.6f | %t | %d / %d |\n", cell(row.Key), cell(result.Error+" "+result.BudgetReason), cell(label), cell(reason), coverage, float64(grade.CostMicros)/1e6, result.InjectionObserved, result.ApprovalProposals, result.WriteExecutions); err != nil {
			return err
		}
	}
	return nil
}
