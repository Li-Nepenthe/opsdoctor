// 本文件：进程重启后，从保存的步骤接回中断的对话。
// restore 取出最后一次真正发出的消息，再接上回答。readTools 复用已保存的结果，只补缺失的查询。

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// restore 的计数来自全部历史，进程重启不能重新获得十五轮或一份新费用预算。
func (run *session) restore(ctx context.Context) (bool, error) {
	latest, finalRounds, err := run.restoreCounters()
	if err != nil || latest == nil {
		return false, err
	}
	var request llm.Request
	var answer llm.Response
	if json.Unmarshal(latest.Args, &request) != nil || json.Unmarshal([]byte(latest.Result), &answer) != nil {
		return false, fmt.Errorf("模型断点消息损坏，无法安全重建")
	}
	run.history, run.parentSeq = request.Messages, latest.Seq
	final := finalRequest(request)
	if final {
		run.result.ReportMode, run.result.BudgetReason = "budget", "恢复最后报告机会"
	}
	if latest.Status != "ok" {
		return run.resumeInterrupted(ctx, final, finalRounds)
	}
	run.lastInput, run.lastBytes = answer.Usage.Input, len(latest.Args)
	run.history = append(run.history, answer.Message)
	done, err := run.calls(ctx, answer.Message.ToolCalls, final)
	if done || err != nil || !final {
		return done, err
	}
	// 最后报告已失败一次时，恢复也只能用剩下的一次修正机会。
	if finalRounds >= 2 || len(answer.Message.ToolCalls) != 1 || answer.Message.ToolCalls[0].Function.Name != "submit_report" {
		run.result.BudgetExceeded = true
		return true, nil
	}
	return run.repairFinal(ctx)
}

func (run *session) restoreCounters() (*Step, int, error) {
	var latest *Step
	finalRounds := 0
	for _, step := range run.input.History {
		run.rememberEvidence(step)
		if step.Kind == "tool" {
			run.completed[step.EventKey] = step
			if step.Counted {
				run.result.ToolCalls++
			}
		}
		if step.Kind != "llm" {
			continue
		}
		run.result.Rounds++
		// 回放沿用已保存步骤的计量口径。未命中缓存的用量保留在单步记录里。
		run.result.Usage.Input += step.Usage.Input
		run.result.Usage.Output += step.Usage.Output
		run.result.Usage.CacheHit += step.Usage.CacheHit
		run.result.Usage.CacheWrite += step.Usage.CacheWrite
		run.result.Usage.Reasoning += step.Usage.Reasoning
		run.result.CostMicros += step.CostMicros
		var request llm.Request
		if err := json.Unmarshal(step.Args, &request); err != nil {
			return nil, 0, fmt.Errorf("模型断点请求损坏：%w", err)
		}
		if finalRequest(request) {
			finalRounds++
		}
		copy := step
		latest = &copy
	}
	// 崩溃时尚未结算的请求仍占费用，不能只累加有成功回复的步骤。
	run.result.CostMicros = max(run.result.CostMicros, run.input.KnownCost)
	return latest, finalRounds, nil
}

func (run *session) resumeInterrupted(ctx context.Context, final bool, finalRounds int) (bool, error) {
	if !final {
		// 中断的普通请求没有可靠回复，下一轮只能从已有证据继续。
		run.history = append(run.history, llm.Message{Role: "user", Content: "上一次模型请求因进程中断没有取得可用回复；费用已保留，请从现有证据继续。"})
		return false, nil
	}
	// 中断的最终请求也占用机会，两次都已用完便直接结束。
	if finalRounds >= 2 {
		run.result.BudgetExceeded = true
		return true, nil
	}
	return run.repairFinal(ctx)
}

// repairFinal 只补一次最终报告，仍沿用原任务剩下的 token、费用与活动时间。
func (run *session) repairFinal(ctx context.Context) (bool, error) {
	repair, err := run.ask(ctx, true)
	if errors.Is(err, llm.ErrDiagnosisBudget) {
		run.result.BudgetExceeded = true
		return true, nil
	}
	if err != nil {
		return false, err
	}
	done, err := run.calls(ctx, repair.Message.ToolCalls, true)
	run.result.BudgetExceeded = !done
	return true, err
}

// finalInstruction 同时用于发送和恢复识别。只用初始输入的诊断也只有报告工具，仅凭菜单无法判断是否已进入预算收尾。
const finalInstruction = "查证预算已用尽。请现在单独调用 submit_report，只使用已经保存的证据；不知道的内容写进 unknowns，不再调用查看工具。"

// finalRequest 同时检查菜单和程序保存的整理指令，不信任模型自称的预算状态。
func finalRequest(request llm.Request) bool {
	if len(request.Tools) != 1 || request.Tools[0].Function.Name != "submit_report" {
		return false
	}
	for _, message := range request.Messages {
		if message.Role == "user" && message.Content == finalInstruction {
			return true
		}
	}
	return false
}
