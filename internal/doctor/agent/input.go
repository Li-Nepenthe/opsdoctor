// 本文件：为初始告警保存证据编号，让没有查询工具的诊断也能交出可核验的报告。
// prepareInput 复用已保存的输入，rememberEvidence 决定这次诊断允许引用哪些记录。

package agent

import (
	"context"
	"encoding/json"
	"fmt"
)

// prepareInput 在有故障开始时间或只用初始输入时保存告警。缺少这两项的历史诊断沿用已存消息。
func (run *session) prepareInput(ctx context.Context) error {
	if run.input.FaultStartedAt.IsZero() && !run.input.InputOnly {
		return nil
	}
	var saved Step
	for _, step := range run.input.History {
		if step.EventKey == "diagnosis-input" && step.Status == "input_evidence" {
			saved = step
			break // 最初保存的输入是比较依据，不能用后来收到的告警覆盖。
		}
	}
	if saved.Seq == 0 {
		var err error
		saved, err = record(ctx, run.input.Recorder, Step{EventKey: "diagnosis-input", Kind: "system", Status: "input_evidence", Args: json.RawMessage(`{}`), Result: run.agent.redactor.Text(run.input.Symptoms)})
		if err != nil {
			return fmt.Errorf("保存初始诊断证据：%w", err)
		}
	}
	run.rememberEvidence(saved)
	// 编码为字符串：告警里的引号和“结束标记”只是数据，不能闭合外层 JSON。
	content, _ := json.Marshal(struct {
		Step int    `json:"input_evidence_step"`
		Data string `json:"untrusted_data"`
	}{saved.Seq, saved.Result})
	run.history[len(run.history)-1].Content = string(content)
	return nil
}

// rememberEvidence 限定报告可引用的范围。只用初始输入时，证据必须来自保存告警的那一步。
func (run *session) rememberEvidence(step Step) {
	if !run.input.InputOnly || (step.EventKey == "diagnosis-input" && step.Status == "input_evidence") {
		run.existing[step.Seq] = true
	}
}
