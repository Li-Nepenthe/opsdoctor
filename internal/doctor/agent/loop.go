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
}

// New 不联网，参数错误在启动时暴露，单元测试可使用较小预算。
func New(client llm.Client, model string, redactor *llm.Redactor, limits Limits) (*Agent, error) {
	if client == nil || model == "" || redactor == nil || limits.Rounds < 1 || limits.Tools < 1 || limits.Tokens < 1 || limits.Duration <= 0 || limits.FinalTime <= 0 || limits.FinalTime >= limits.Duration {
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
	run := &session{agent: agent, input: input, history: messages(input, agent.redactor), definitions: definitions(input), existing: map[int]bool{}}
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
	agent := run.agent
	for run.result.Rounds < agent.limits.Rounds && run.result.ToolCalls < agent.limits.Tools && run.result.Usage.Input+run.result.Usage.Output < agent.limits.Tokens {
		answer, err := run.ask(work, false)
		if err != nil {
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
		run.history = append(run.history, answer.Message)
	}
	return answer, callErr
}

// finish 只给一次机会；仍然索要新工具、提交错误报告或没有提交，都标为超出预算。
func (run *session) finish(ctx context.Context) (Result, error) {
	if ctx.Err() != nil {
		return run.result, ctx.Err()
	}
	run.history = append(run.history, llm.Message{Role: "user", Content: "查证预算已用尽。请现在单独调用 submit_report，只使用已经保存的证据；不知道的内容写进 unknowns，不再调用查看工具。"})
	answer, err := run.ask(ctx, true)
	if err != nil {
		return run.result, err
	}
	done, err := run.calls(ctx, answer.Message.ToolCalls, true)
	if err != nil {
		return run.result, err
	}
	run.result.BudgetExceeded = !done
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
