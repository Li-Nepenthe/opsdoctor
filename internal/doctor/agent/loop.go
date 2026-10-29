// 本文件：控制诊断从第一次询问模型到提交报告的整个过程。
//
// 循环不是“模型说什么就做什么”。每轮先检查预算；调用完先记录用量和步骤；
// 只有通过报告校验才算成功。普通工具失败会作为数据回给模型，审计失败则必须停止。
//
// 主要内容：New 固定依赖；Run 管理总时限；ask 保存模型步骤；finish 给最后一次报告机会。
// 阅读建议：预算触发后的那次机会只允许 submit_report，不能继续搜集新证据。

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// Agent 构造后只读；每次 Run 的消息和计数都放在独立 session 中。
type Agent struct {
	client   llm.Client    // 真实或脚本模型，调用方式相同。
	model    string        // 本次进程配置的初始模型名。
	redactor *llm.Redactor // 所有边界内容共用的脱敏规则。
	limits   Limits        // 正常阶段的预算和总时限。
}

// session 保存一次诊断的状态，不把历史挂在共享模型客户端上。
type session struct {
	agent       *Agent        // 构造依赖。
	input       Input         // 固定系统与记录器。
	history     []llm.Message // 每轮只追加，不改固定前缀。
	definitions []llm.Tool    // 本系统工具加 submit_report。
	existing    map[int]bool  // 本次已经成功保存的步骤编号。
	result      Result        // 用量与报告累计。
	lastInput   int64         // 上一轮供应商计量，作为下一轮估计历史的依据。
	lastBytes   int           // 上一轮发送的 JSON 大小，新增内容单独计算。
}

// New 不联网，参数错误在启动时暴露，单元测试可使用较小预算。
func New(client llm.Client, model string, redactor *llm.Redactor, limits Limits) (*Agent, error) {
	// 旧测试只缩小轮数或总量时，新边界仍默认开启，不能因零值悄悄失去保护。
	if limits.ContextTokens == 0 {
		limits.ContextTokens = DefaultLimits().ContextTokens
	}
	if limits.CostMicros == 0 {
		limits.CostMicros = DefaultLimits().CostMicros
	}
	if client == nil || model == "" || redactor == nil || limits.Rounds < 1 || limits.Tools < 1 || limits.Tokens < 1 || limits.ContextTokens < 1 || limits.CostMicros < 1 || limits.Duration <= 0 || limits.FinalTime <= 0 || limits.FinalTime >= limits.Duration {
		return nil, fmt.Errorf("Agent 依赖或预算不完整")
	}
	return &Agent{client: client, model: model, redactor: redactor, limits: limits}, nil
}

// Run 为普通查证和最后整理分别设截止时间，但二者加起来仍只有一份总时限。
func (agent *Agent) Run(parent context.Context, input Input) (Result, error) {
	if input.ID == "" || input.Tools == nil || input.Recorder == nil || len(input.Components) == 0 {
		return Result{}, fmt.Errorf("诊断缺少系统、工具或审计依赖")
	}
	whole, cancel := context.WithTimeout(parent, agent.limits.Duration)
	defer cancel()
	work, stop := context.WithTimeout(whole, agent.limits.Duration-agent.limits.FinalTime)
	defer stop()
	run := &session{agent: agent, input: input, history: messages(input, agent.redactor), definitions: definitions(input), existing: map[int]bool{}, result: Result{ReportMode: "voluntary"}}
	result, err := run.execute(whole, work)
	// 总时限耗尽属于预算终态，主动取消仍保留 context.Canceled 给调度器区分。
	if parent.Err() == nil && errors.Is(whole.Err(), context.DeadlineExceeded) {
		result.BudgetExceeded = true
		return result, nil
	}
	return result, err
}

// execute 在每次回答之后检查累计用量，最后报告机会仍受 whole 的总时限约束。
func (run *session) execute(whole, work context.Context) (Result, error) {
	for run.budgetReason() == "" {
		answer, err := run.ask(work, false)
		if err != nil {
			if errors.Is(err, llm.ErrDiagnosisBudget) {
				if run.result.BudgetReason == "" {
					run.result.BudgetReason = err.Error()
				}
				return run.finish(whole)
			}
			// 普通阶段时间耗尽后还留有报告时间；主动取消和额度不足不能再发一次付费请求。
			if work.Err() != nil && whole.Err() == nil {
				return run.finish(whole)
			}
			return run.result, err
		}
		done, err := run.calls(work, answer.Message.ToolCalls, false)
		if err != nil {
			if work.Err() != nil && whole.Err() == nil {
				return run.finish(whole)
			}
			return run.result, err
		}
		if done {
			return run.result, nil
		}
	}
	return run.finish(whole)
}

// ask 把实际脱敏后的请求写进步骤；真实 HTTP 尝试另由 llm 账本在发送前保存。
func (run *session) ask(ctx context.Context, final bool) (llm.Response, error) {
	request := llm.Request{DiagnosisID: run.input.ID, Model: run.agent.model, Messages: run.history, Tools: run.definitions, MaxTokens: 4096, Thinking: llm.Thinking{Type: "disabled"}}
	if final {
		request.Tools = []llm.Tool{reportDefinition(run.input.Components)}
	}
	raw, _ := json.Marshal(request)
	clean, err := run.agent.redactor.JSON(raw)
	if err != nil {
		return llm.Response{}, err
	}
	// 再解析得到模型真正看到的内容，内存脚本与真实 HTTP 不走两套脱敏路径。
	if err := json.Unmarshal(clean, &request); err != nil {
		return llm.Response{}, err
	}
	request.DiagnosisID = run.input.ID
	request.DiagnosisLimitMicros = run.agent.limits.CostMicros
	request.InputUpperBound = run.inputBound(len(clean))
	requestTokens := request.InputUpperBound + int64(request.MaxTokens)
	if requestTokens > run.agent.limits.ContextTokens {
		run.result.BudgetReason = "单次请求上下文达到上限"
		return llm.Response{}, llm.ErrDiagnosisBudget
	}
	remaining := run.agent.limits.Tokens - run.result.Usage.Input - run.result.Usage.Output
	// 累计量也是硬边界，最后的报告和修正不能越过它。普通阶段提前留一份报告额度。
	if requestTokens > remaining || (!final && run.result.Rounds > 0 && 2*requestTokens > remaining) {
		run.result.BudgetReason = "累计 token 余额不足，预留最后报告空间"
		return llm.Response{}, llm.ErrDiagnosisBudget
	}
	if run.result.CostMicros >= run.agent.limits.CostMicros {
		return llm.Response{}, llm.ErrDiagnosisBudget
	}
	started := time.Now()
	answer, callErr := run.agent.client.Complete(ctx, request)
	run.result.Rounds++
	run.result.Usage.Input += answer.Usage.Input
	run.result.Usage.Output += answer.Usage.Output
	run.result.Usage.CacheHit += answer.Usage.CacheHit
	run.result.Usage.CacheMiss += answer.Usage.CacheMiss
	run.result.CostMicros += answer.CostMicros
	if answer.Model == "" {
		answer.Model = run.agent.model
	}
	response, _ := json.Marshal(answer)
	response, err = run.agent.redactor.JSON(response)
	if err != nil {
		return answer, err
	}
	_ = json.Unmarshal(response, &answer)
	status := "ok"
	if callErr != nil {
		status = "failed"
	}
	if errors.Is(callErr, context.Canceled) {
		status = "cancelled"
	}
	step, recordErr := record(ctx, run.input.Recorder, Step{Kind: "llm", Model: answer.Model, Args: clean, Result: string(response), Status: status, DurationMS: time.Since(started).Milliseconds(), Usage: answer.Usage, CostMicros: answer.CostMicros})
	if recordErr != nil {
		return answer, fmt.Errorf("保存模型步骤：%w", recordErr)
	}
	run.existing[step.Seq] = true
	if callErr == nil {
		run.lastInput, run.lastBytes = answer.Usage.Input, len(clean)
		run.history = append(run.history, answer.Message)
	}
	return answer, callErr
}

// finish 先要求整理；如果确实提交了报告但校验失败，再给一次修正机会。
// 修正仍受总时间、上下文和费用约束，不会因为 JSON 错误获得无限次免费额度。
func (run *session) finish(ctx context.Context) (Result, error) {
	if ctx.Err() != nil {
		return run.result, ctx.Err()
	}
	run.result.ReportMode = "budget"
	if run.result.BudgetReason == "" {
		run.result.BudgetReason = run.budgetReason()
	}
	if run.result.BudgetReason == "" {
		run.result.BudgetReason = "查证时间用尽"
	}
	run.history = append(run.history, llm.Message{Role: "user", Content: "查证预算已用尽。请现在单独调用 submit_report，只使用已经保存的证据；不知道的内容写进 unknowns，不再调用查看工具。"})
	for attempt := 0; attempt < 2; attempt++ {
		answer, err := run.ask(ctx, true)
		if errors.Is(err, llm.ErrDiagnosisBudget) {
			run.result.BudgetExceeded = true
			return run.result, nil
		}
		if err != nil {
			return run.result, err
		}
		done, err := run.calls(ctx, answer.Message.ToolCalls, true)
		if err != nil {
			return run.result, err
		}
		if done {
			return run.result, nil
		}
		// 只修正已提交报告的校验错误；继续索要查询或只说普通文字，不算报告修正。
		if len(answer.Message.ToolCalls) != 1 || answer.Message.ToolCalls[0].Function.Name != "submit_report" {
			break
		}
	}
	run.result.BudgetExceeded = true
	return run.result, nil
}

// record 取消之后只留半秒保存最后状态；真实查询仍使用已取消的原 context。
func record(parent context.Context, recorder Recorder, step Step) (Step, error) {
	const saveTimeout = 500 * time.Millisecond // 加上模型结算的半秒，给两秒取消承诺留出调度余量。
	bounded, cancel := context.WithTimeout(context.WithoutCancel(parent), saveTimeout)
	defer cancel()
	return recorder.Record(bounded, step)
}

// markMock 在程序端加醒目标记，不能靠脚本模型自己记住声明。
func markMock(raw json.RawMessage) json.RawMessage {
	var value map[string]any
	_ = json.Unmarshal(raw, &value)
	value["summary"] = "Mock 模式，不是真实诊断。" + fmt.Sprint(value["summary"])
	value["unknowns"] = "这是固定脚本演示，不能作为真实根因结论。" + fmt.Sprint(value["unknowns"])
	result, _ := json.Marshal(value)
	return result
}

// isMock 只检查程序选定的模型名，供应商回复不能把真实请求变成免费请求。
func (agent *Agent) isMock() bool { return strings.HasPrefix(agent.model, "mock") }
