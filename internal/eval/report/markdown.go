// 本文件：把已保存的结果、评分和失败排入 Markdown 表格。未完成的槽位保留原位置，金额附上币种，费用取自用量账本。

package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
)

// Markdown 不在导出时重新评分，相同数据库快照应当生成相同的表格。
func Markdown(output io.Writer, run store.EvalRun, rows []store.EvalResult) error {
	origin := "自建靶场"
	if run.Target != "" && run.Target != "shop" {
		origin = "接入系统 " + cell(run.Target)
	}
	if _, err := fmt.Fprintf(output, "# 评测记录 %s\n\n这张表来自%s。请按批次配置区分试跑和正式评测；每个场景样本很少，不能代表生产准确率。美元按项目用量账本计：每次请求实际 token 用量乘以 2026-10-29 核对过的官方单价；分档结算，未知请求保留预留。平台账单因 Key 与其他用途共用，无法单独核对。\n\n批次 token：%d / %d（含未确认预留）。\n", cell(run.ID), origin, run.TokensUsed, run.TokenLimit); err != nil {
		return err
	}
	// 批次身份和槽位身份都能触发阶段 9 门槛，旧槽位缺字段时也不能漏过审计。
	requireAudit := run.Target == "otel-demo" || strings.HasPrefix(run.ID, "phase09-")
	qualityTables := requireAudit
	for _, row := range rows {
		qualityTables = qualityTables || requiresCollectionAudit(row)
	}
	if qualityTables {
		if err := collectionRows(output, rows, requireAudit); err != nil {
			return err
		}
	} else if err := resultRows(output, rows); err != nil {
		return err
	}
	if err := statistics(output, rows, requireAudit); err != nil {
		return err
	}
	return gradingRows(output, rows, requireAudit)
}

// resultRows 只展示实际记录；采集受影响的报告也保留原组件判断、用量和费用。
func resultRows(output io.Writer, rows []store.EvalResult) error {
	if _, err := fmt.Fprintln(output, "\n| 场景 | 轮次 | 组 | 诊断 | 状态 | 组件一致 | 轮数 | 工具 | 输入 / 输出 token | 账本费用 | 耗时秒 | 提交方式 | 混合模型 |\n|---|---:|---|---|---|---|---:|---:|---:|---:|---:|---|---|"); err != nil {
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
	return nil
}

// collectionRows 把正常、已受影响和无法验证分成独立表格，不能靠翻到脚注才发现排除项。
func collectionRows(output io.Writer, rows []store.EvalResult, requireAudit bool) error {
	groups := map[string][]store.EvalResult{}
	// 先分组再画表；原诊断的正确与否不会改变它所属的采集质量组。
	for _, row := range rows {
		var value Summary
		if len(row.Data) > 0 && json.Unmarshal(row.Data, &value) != nil {
			return fmt.Errorf("采集质量结果无法解码")
		}
		quality := QualityNormal
		if requireAudit || requiresCollectionAudit(row) {
			quality = CollectionQuality(value)
		}
		groups[quality] = append(groups[quality], row)
	}
	for _, group := range []struct{ quality, title string }{
		{QualityNormal, "采集正常样本"},
		{QualityAffected, "采集受影响样本（不计入正常统计）"},
		{QualityUnknown, "采集质量未知样本（不计入正常统计）"},
	} {
		if _, err := fmt.Fprintf(output, "\n## %s\n", group.title); err != nil {
			return err
		}
		if err := resultRows(output, groups[group.quality]); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(output, "\n## 采集窗口审计\n\nnormal 需要完整且成功的窗口审计。affected 表示已观察到最终丢弃或映射错误；unknown 表示没有审计或审计无法确认。原诊断及全部费用仍保留，受影响或未知样本的评分如已存在，也只作原始记录。\n\n| 槽位 | 采集质量 | 窗口 UTC | 最终丢弃事件 | OpenSearch 日志 dropped_items | mapper 行 / 次数 | 各信号 dropped_items / 事件 | 原始字节 | SHA256 | 原始日志路径 | 审计状态 / 容器 / 错误 |\n|---|---|---|---|---|---|---|---|---|---|---|"); err != nil {
		return err
	}
	for _, row := range rows {
		var value Summary
		if len(row.Data) > 0 && json.Unmarshal(row.Data, &value) != nil {
			return fmt.Errorf("采集窗口审计无法解码")
		}
		quality := CollectionQuality(value)
		if !requireAudit && !requiresCollectionAudit(row) {
			quality = "不适用（阶段 8）"
		}
		audit := value.CollectorAudit
		if audit == nil {
			if _, err := fmt.Fprintf(output, "| %s | %s | 未验证 | — | — | — | — | — | — | — | 无审计 |\n", cell(row.Key), cell(quality)); err != nil {
				return err
			}
			continue
		}
		signals, _ := json.Marshal(map[string]map[string]int64{"dropped_items": audit.SignalDroppedItems, "dropping_data_events": audit.SignalDroppingDataEvents})
		window := audit.WindowStart.UTC().Format(time.RFC3339Nano) + " → " + audit.WindowEnd.UTC().Format(time.RFC3339Nano)
		if _, err := fmt.Fprintf(output, "| %s | %s | %s | %d | %d | %d / %d | %s | %d | %s | %s | %s |\n", cell(row.Key), cell(quality), cell(window), audit.DroppingDataEvents, audit.LogDroppedItems, audit.MapperErrorLines, audit.MapperErrorOccurrences, cell(string(signals)), audit.RawBytes, cell(audit.SHA256), cell(audit.RawPath), cell(audit.Status+" "+audit.ContainerID+" "+audit.Error)); err != nil {
			return err
		}
	}
	return nil
}

// cell 只整理表格分隔符，不改报告的数据库原文。
func cell(text string) string {
	return strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ").Replace(text)
}

// gradingRows 保留每份机制理由、评分失败和错误说明，独立盲评之后再补“评分有误”的分析。
func gradingRows(output io.Writer, rows []store.EvalResult, requireAudit bool) error {
	// 显示原理由而不自动推定失败原因，独立盲评可据完整步骤重新核对。
	fmt.Fprintln(output, "\n## 评分与失败明细\n\n以下是原始结论与评委理由；评委也是模型，未经独立复核，不能视为标准答案。格式、预算、可靠性和评分失败分别显示。推理错误、工具不足、数据缺失以及评分有误的最终分类，需要结合原步骤逐条核对。\n\n| 槽位 | 医生错误 / 预算原因 | 机制 / 评分状态 | 评分理由 | 证据覆盖 | 评委账本 | S12 实际暴露 | 写提议 / 执行 |\n|---|---|---|---|---|---|---|---|")
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
		label += collectionGradeNote(row, result, requireAudit)
		if _, err := fmt.Fprintf(output, "| %s | %s | %s | %s | %s | CNY %.6f | %t | %d / %d |\n", cell(row.Key), cell(result.Error+" "+result.BudgetReason), cell(label), cell(reason), coverage, float64(grade.CostMicros)/1e6, result.InjectionObserved, result.ApprovalProposals, result.WriteExecutions); err != nil {
			return err
		}
	}
	return nil
}

// collectionGradeNote 给未通过窗口审计的槽位注明分母口径，已有的评分仍可查看。
func collectionGradeNote(row store.EvalResult, result Summary, requireAudit bool) string {
	if !requireAudit && !requiresCollectionAudit(row) {
		return ""
	}
	quality := CollectionQuality(result)
	if quality == QualityNormal {
		return ""
	}
	note := "不计入正常统计"
	if len(row.Judgement) > 0 {
		note = "仅保留原评分"
	}
	return "（采集 " + quality + "；" + note + "）"
}
