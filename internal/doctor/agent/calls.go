// 本文件：执行一轮工具请求，并把结果按模型给出的 ID 放回对话。
//
// 彼此独立的只读查询可以同时进行，但最多四个。一个查询失败时，其他查询仍
// 可能拿到有用证据，所以普通工具失败不作为 errgroup 的错误返回。
//
// 主要内容：calls 处理报告与混合请求；readTools 并行执行；saveTool 为结果分配证据步骤号。
// 阅读建议：切片每格只由一个 goroutine 写，Wait 之后主循环才读取，避免共享追加造成竞态。

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
	"golang.org/x/sync/errgroup"
)

// calls 要求报告独占一轮；混合时执行查看工具，明确拒绝报告，让模型看完新结果再提交。
func (run *session) calls(ctx context.Context, calls []llm.ToolCall, final bool) (bool, error) {
	// 普通文字不算报告，必须经过同一套结构和证据检查才能结束。
	if len(calls) == 0 {
		run.history = append(run.history, llm.Message{Role: "user", Content: "请调用查看工具获取证据，或单独调用 submit_report 提交报告。普通文字不会结束诊断。"})
		return false, nil
	}
	if err := validateCalls(calls); err != nil {
		return false, err
	}
	// 写操作必须独占一轮。这样人审批时看到的依据不会与同轮尚未返回的查询混在一起。
	if len(calls) == 1 && run.input.Approvals != nil && run.input.Approvals.Handles(calls[0].Function.Name) && !final {
		return run.writeTool(ctx, calls[0])
	}
	if len(calls) == 1 && calls[0].Function.Name == "submit_report" {
		raw, err := validateReport([]byte(calls[0].Function.Arguments), run.input.Components, run.existing)
		if err == nil {
			if run.agent.isMock() {
				raw = markMock(raw)
			}
			message, step, saveErr := run.saveTool(ctx, calls[0], tools.Result{Tool: "submit_report", Status: "ok", Summary: "报告已通过校验，建议操作仅记录", Data: json.RawMessage(`null`)}, false)
			if saveErr != nil {
				return false, saveErr
			}
			run.history = append(run.history, message)
			run.existing[step.Seq] = true
			run.result.Report = raw
			return true, nil
		}
		message, step, saveErr := run.saveTool(ctx, calls[0], invalidReport(err.Error()), false)
		if saveErr != nil {
			return false, saveErr
		}
		run.history = append(run.history, message)
		run.existing[step.Seq] = true
		return false, nil
	}
	return false, run.readTools(ctx, calls, final)
}

// validateCalls 只检查协议关联，工具名是否存在和参数是否正确仍交给统一注册表。
func validateCalls(calls []llm.ToolCall) error {
	if len(calls) > 40 {
		return fmt.Errorf("单轮模型工具请求超过四十项")
	}
	seen := map[string]bool{}
	for _, call := range calls {
		if call.ID == "" || len(call.ID) > 128 || seen[call.ID] || call.Type != "function" {
			return fmt.Errorf("模型工具调用 ID 缺失、重复或类型不支持")
		}
		seen[call.ID] = true
	}
	return nil
}

// readTools 仅审计失败才中止整轮；参数错误、超时和数据源错误都作为普通工具结果。
func (run *session) readTools(ctx context.Context, calls []llm.ToolCall, final bool) error {
	const parallelTools = 4 // 计划书上限，MySQL 的三连接池还会进一步排队。
	group := new(errgroup.Group)
	group.SetLimit(parallelTools)
	replies := make([]llm.Message, len(calls))
	steps := make([]Step, len(calls))
	for index, call := range calls {
		if saved, ok := run.completed[toolKey(run.parentSeq, call.ID)]; ok {
			// 回复已落库时直接回放，不再次查数据，更不能再次执行写操作。
			replies[index], steps[index] = toolMessage(saved, call.ID), saved
			continue
		}
		if ctx.Err() != nil {
			break
		} // 已取消时不再排新的查询，先等在途查询退出。
		allowed := !final && run.result.ToolCalls < run.agent.limits.Tools
		if allowed {
			run.result.ToolCalls++
		}
		// 每个任务只写自己的切片元素；group.Wait 等所有保存结束后才合并历史。
		group.Go(func() error {
			var result tools.Result
			switch {
			case call.Function.Name == "submit_report":
				result = invalidReport("submit_report 必须单独一轮；本轮查看工具结果返回后再提交")
			case run.input.Approvals != nil && run.input.Approvals.Handles(call.Function.Name):
				result = tools.Result{Tool: call.Function.Name, Status: "invalid_argument", Summary: "写工具必须单独一轮申请；不能与其他工具或报告混在一起", Data: json.RawMessage(`null`)}
			case !allowed:
				result = tools.Result{Tool: call.Function.Name, Status: "invalid_argument", Summary: "查证预算已用尽，只能单独提交报告", Data: json.RawMessage(`null`)}
			default:
				result = run.input.Tools.Run(ctx, call.Function.Name, json.RawMessage(call.Function.Arguments))
			}
			message, step, err := run.saveTool(ctx, call, result, allowed)
			replies[index], steps[index] = message, step
			return err
		})
	}
	if err := group.Wait(); err != nil {
		return fmt.Errorf("保存工具步骤：%w", err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for index := range replies {
		run.history = append(run.history, replies[index])
		run.existing[steps[index].Seq] = true
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// saveTool 把实际步骤号包在 tool 回复里，模型无需猜测并行完成的顺序。
func (run *session) saveTool(ctx context.Context, call llm.ToolCall, result tools.Result, counted bool) (llm.Message, Step, error) {
	key := toolKey(run.parentSeq, call.ID)
	if saved, ok := run.completed[key]; ok {
		return toolMessage(saved, call.ID), saved, nil
	}
	started := time.Now()
	raw, _ := json.Marshal(result)
	clean, err := run.agent.redactor.JSON(raw)
	if err != nil {
		return llm.Message{}, Step{}, err
	}
	args, err := run.agent.redactor.JSON([]byte(call.Function.Arguments))
	if err != nil {
		// 无效 JSON 仍需留审计；把原文当字符串保存，不能让错误参数阻止循环纠错。
		args, _ = json.Marshal(run.agent.redactor.Text(call.Function.Arguments))
	}
	step, err := record(ctx, run.input.Recorder, Step{EventKey: key, ParentSeq: run.parentSeq, CallID: call.ID, Counted: counted, Kind: "tool", ToolName: result.Tool, Args: args, Result: string(clean), Status: result.Status, DurationMS: result.DurationMS + time.Since(started).Milliseconds()})
	if err != nil {
		return llm.Message{}, step, err
	}
	return toolMessage(step, call.ID), step, nil
}

// toolKey 加入模型步骤号；供应商在不同轮里复用同一个 ID 时仍然是两次不同调用。
func toolKey(parent int, id string) string { return fmt.Sprintf("tool:%d:%s", parent, id) }

// toolMessage 把工具正文编码成 JSON 字符串。正文里的引号和伪造结束标记只会成为数据，
// 无法闭合外层结构；系统提示仍要求模型不要把这份数据当作指令。
func toolMessage(step Step, callID string) llm.Message {
	content, _ := json.Marshal(struct {
		Step int    `json:"step"`
		Data string `json:"untrusted_data"`
	}{step.Seq, step.Result})
	return llm.Message{Role: "tool", ToolCallID: callID, Content: string(content)}
}

// invalidReport 与普通工具错误一样交回模型，只说明校验失败，不结束诊断。
func invalidReport(reason string) tools.Result {
	return tools.Result{Tool: "submit_report", Status: "invalid_argument", Summary: reason, Data: json.RawMessage(`null`)}
}

// writeTool 等待时不伪造工具完成结果。批准、拒绝或过期后恢复，才把真实结果接回原 call_id。
func (run *session) writeTool(ctx context.Context, call llm.ToolCall) (bool, error) {
	key := toolKey(run.parentSeq, call.ID)
	if saved, ok := run.completed[key]; ok {
		run.history = append(run.history, toolMessage(saved, call.ID))
		return false, nil
	}
	if run.result.ToolCalls >= run.agent.limits.Tools {
		return false, fmt.Errorf("写工具申请已超过工具预算")
	}
	// 暂停时不记作已完成调用，恢复后仍要把结果回给同一个 tool_call_id。
	waiting, result, err := run.input.Approvals.Handle(ctx, run.input.ID, run.parentSeq, call, run.existing)
	if err != nil {
		return false, err
	}
	if waiting {
		run.result.WaitingApproval = true
		return true, nil
	}
	run.result.ToolCalls++
	message, step, err := run.saveTool(ctx, call, result, true)
	if err != nil {
		return false, err
	}
	run.history = append(run.history, message)
	run.existing[step.Seq] = true
	return false, nil
}
