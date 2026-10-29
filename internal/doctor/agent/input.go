// 本文件：把最初的告警输入也保存为一个可引用的审计步骤。
//
// 没有工具的实验组只能根据告警判断。它仍需引用真实存在的证据，不能靠随意填写
// 步骤号通过报告校验。输入先脱敏、落库，再把数据库分配的编号交给模型。
//
// 主要内容：prepareInput 保存或找回输入步骤；rememberEvidence 限制无工具组的证据来源。
// 阅读建议：恢复沿用原步骤，新的通知不会偷偷改掉这一轮比较使用的初始输入。

package agent

import (
	"context"
	"encoding/json"
	"fmt"
)

// prepareInput 只在提供故障时间或启用输入组时记录，旧诊断的恢复格式保持可读。
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

// rememberEvidence 在 E0 下拒绝把模型自己说的话当作证据，其他诊断仍沿用原来的步骤校验。
func (run *session) rememberEvidence(step Step) {
	if !run.input.InputOnly || (step.EventKey == "diagnosis-input" && step.Status == "input_evidence") {
		run.existing[step.Seq] = true
	}
}
