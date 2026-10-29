// 本文件：从已保存的模型请求和工具结果，重建中断前的对话。
//
// 每次模型步骤都保存了实际发送的全部消息。恢复时取最后一份，再接回这次回答和
// 已经完成的工具结果。这样不需要猜测旧提示，也不会丢掉原来的固定前缀。
//
// 主要内容：restore 恢复计数和消息；finalRequest 判断最后一轮是否只允许交报告。
// 阅读建议：没有结果的只读查询可以重查；已经保存的结果必须原样回放。

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
	var latest *Step
	finalRounds := 0
	for _, step := range run.input.History {
		run.existing[step.Seq] = true
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
		run.result.Usage.Input += step.Usage.Input
		run.result.Usage.Output += step.Usage.Output
		run.result.Usage.CacheHit += step.Usage.CacheHit
		run.result.Usage.CacheWrite += step.Usage.CacheWrite
		run.result.Usage.Reasoning += step.Usage.Reasoning
		run.result.CostMicros += step.CostMicros
		var request llm.Request
		if err := json.Unmarshal(step.Args, &request); err != nil {
			return false, fmt.Errorf("模型断点请求损坏：%w", err)
		}
		if finalRequest(request) {
			finalRounds++
		}
		copy := step
		latest = &copy
	}
	// 崩溃时尚未结算的请求仍占费用，不能只累加有成功回复的步骤。
	run.result.CostMicros = max(run.result.CostMicros, run.input.KnownCost)
	if latest == nil {
		return false, nil
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
		// 最后整理中断也消耗一次机会，不能重启后又回到普通查证。
		if final {
			if finalRounds >= 2 {
				run.result.BudgetExceeded = true
				return true, nil
			}
			return run.repairFinal(ctx)
		}
		// 未收到可靠回复的请求没有可执行的工具；明确告知下一轮已有一次中断。
		run.history = append(run.history, llm.Message{Role: "user", Content: "上一次模型请求因进程中断没有取得可用回复；费用已保留，请从现有证据继续。"})
		return false, nil
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

// finalRequest 由程序保存的工具菜单识别整理阶段，不信任模型自称的预算状态。
func finalRequest(request llm.Request) bool {
	return len(request.Tools) == 1 && request.Tools[0].Function.Name == "submit_report"
}
