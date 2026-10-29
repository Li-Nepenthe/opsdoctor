// 本文件：用脚本模型和内存记录器验证完整诊断循环。
//
// 测试自己决定每轮模型回复，因此可以稳定复现编造证据、混合报告、超预算和取消。
// 所有数据都在内存中，不读取 Key，不连接真实模型或业务系统。
//
// 主要内容：TestLoopCorrection、TestMixedReport 检查纠错；TestBudget、TestTimeBudget
// 检查次数和时间上限；TestParallelCancel 检查并发取消；TestArgumentCorrection 检查参数纠错。
// 阅读建议：先看脚本收到上一轮 tool 消息时怎样检查 step 和 tool_call_id。

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// modelFunc 让每个测试直接写对话脚本，不在生产客户端里加入测试开关。
type modelFunc func(context.Context, llm.Request) (llm.Response, error)

// Complete 只调用本测试提供的函数。
func (function modelFunc) Complete(ctx context.Context, request llm.Request) (llm.Response, error) {
	return function(ctx, request)
}

// memoryRecorder 用一把锁保护步骤号和内容，模拟并行工具完成后的顺序分配。
type memoryRecorder struct {
	mu    sync.Mutex
	steps []Step
}

// Record 只在确实追加后返回编号，报告不能引用尚未保存的步骤。
func (recorder *memoryRecorder) Record(_ context.Context, step Step) (Step, error) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	step.Seq = len(recorder.steps) + 1
	recorder.steps = append(recorder.steps, step)
	return step, nil
}

// toolFunc 保留统一工具结果格式，测试可让某个查询等待取消或返回参数错误。
type toolFunc func(context.Context, string, json.RawMessage) tools.Result

// List 返回固定工具说明，调用计数来自 Run，不能用列菜单替代实际查询。
func (function toolFunc) List() []tools.Definition {
	return []tools.Definition{{Name: "query", Description: "测试查看", ReadOnly: true, Parameters: tools.Schema{Type: "object"}}}
}

// Run 将控制交给测试，仍使用诊断的同一个 context。
func (function toolFunc) Run(ctx context.Context, name string, args json.RawMessage) tools.Result {
	return function(ctx, name, args)
}

// call 为脚本生成合法关联 ID，参数本身仍可以故意写错。
func call(id, name, args string) llm.ToolCall {
	return llm.ToolCall{ID: id, Type: "function", Function: llm.Function{Name: name, Arguments: args}}
}

// answer 提供实际模型名和用量，检查每一轮都被记录而不是只记首轮。
func answer(calls ...llm.ToolCall) llm.Response {
	return llm.Response{Model: "actual-test-model", Message: llm.Message{Role: "assistant", ToolCalls: calls}, Usage: llm.Usage{Input: 10, Output: 2}, CostMicros: 36}
}

// reportArgs 可以引用一次真实查看，也可用零表示没有足够证据的报告。
func reportArgs(seq int) string {
	evidence := "[]"
	if seq > 0 {
		evidence = fmt.Sprintf(`[{"step":%d,"observation":"测试工具返回了一条观察"}]`, seq)
	}
	return `{"summary":"测试结论","root_cause":{"component":"unknown","mechanism":"证据不足","confidence":0.1},"evidence":` + evidence + `,"suggested_actions":[{"tool":"manual","args":{},"risk":"需要人核对"}],"unknowns":"仍需核对"}`
}

// testInput 把系统、组件和记录器显式绑定，和生产一样没有可切换系统的工具参数。
func testInput(recorder *memoryRecorder, toolbox Toolbox) Input {
	return Input{ID: "test-id", Profile: "测试服务架构", Components: []string{"unknown", "api"}, Unavailable: []string{"日志"}, Symptoms: "password=demo-secret-value", Tools: toolbox, Recorder: recorder}
}

// TestLoopCorrection 先调用不存在的工具，再查证据，编造步骤被退回后模型修正报告。
func TestLoopCorrection(t *testing.T) {
	// 逐轮检查纠错结果，不能只断言最终有报告。
	recorder := &memoryRecorder{}
	registry := tools.New(tools.Dependencies{Profile: "一个服务"})
	count := 0
	var prefix []llm.Message
	model := modelFunc(func(_ context.Context, request llm.Request) (llm.Response, error) {
		count++
		if count == 1 {
			prefix = append([]llm.Message{}, request.Messages[:2]...)
		}
		if request.Messages[0].Content != prefix[0].Content || request.Messages[1].Content != prefix[1].Content || !strings.Contains(prefix[0].Content, "日志") {
			t.Fatal("固定前缀变化或缺少不可用数据源")
		}
		raw, _ := json.Marshal(request)
		if strings.Contains(string(raw), "demo-secret-value") {
			t.Fatal("未脱敏的症状发给了模型")
		}
		switch count {
		case 1:
			return answer(call("missing", "missing", "{}")), nil
		case 2:
			if last := request.Messages[len(request.Messages)-1]; last.ToolCallID != "missing" || !strings.Contains(last.Content, "invalid_argument") {
				t.Fatal("未知工具错误未交回")
			}
			return answer(call("topology", "service_topology", "{}")), nil
		case 3:
			return answer(call("bad-report", "submit_report", reportArgs(999))), nil
		case 4:
			if !strings.Contains(request.Messages[len(request.Messages)-1].Content, "不存在") {
				t.Fatal("编造步骤未退回")
			}
			// 取真实保存的查看步骤；发送前断点也占序号，不能硬编码某个数字。
			seq := 0
			for _, step := range recorder.steps {
				if step.ToolName == "service_topology" {
					seq = step.Seq
				}
			}
			return answer(call("good-report", "submit_report", reportArgs(seq))), nil
		default:
			t.Fatal("报告成功后仍在调用模型")
			return llm.Response{}, nil
		}
	})
	agent, err := New(model, "mock-test", llm.NewRedactor(), DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	result, err := agent.Run(t.Context(), testInput(recorder, registry))
	if err != nil || result.Rounds != 4 || result.Usage.Input != 40 || result.CostMicros != 144 || !strings.Contains(string(result.Report), "Mock 模式，不是真实诊断") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for _, step := range recorder.steps {
		if step.Kind == "llm" && step.Model != "actual-test-model" {
			t.Fatal("模型名丢失")
		}
		if strings.Contains(string(step.Args)+step.Result, "demo-secret-value") {
			t.Fatal("审计保存了未脱敏内容")
		}
	}
}

// TestMixedReport 模型不能一边索要新证据，一边抢先用旧证据结束诊断。
func TestMixedReport(t *testing.T) {
	// 逐轮检查纠错结果，不能只断言最终有报告。
	recorder := &memoryRecorder{}
	count := 0
	model := modelFunc(func(_ context.Context, request llm.Request) (llm.Response, error) {
		count++
		if count == 1 {
			return answer(call("query", "service_topology", "{}"), call("premature", "submit_report", reportArgs(0))), nil
		}
		last := request.Messages[len(request.Messages)-1]
		if last.ToolCallID != "premature" || !strings.Contains(last.Content, "必须单独") {
			t.Fatal("混合报告没有被明确退回")
		}
		return answer(call("report", "submit_report", reportArgs(0))), nil
	})
	agent, _ := New(model, "mock-test", llm.NewRedactor(), DefaultLimits())
	result, err := agent.Run(t.Context(), testInput(recorder, tools.New(tools.Dependencies{Profile: "架构"})))
	if err != nil || result.Report == nil || count != 2 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

// TestBudget 每种预算触发后只给一次交报告机会，最后仍请求查询也不会真的执行。
func TestBudget(t *testing.T) {
	// 分别只耗尽一种预算，确认最后一次机会不执行新的查看。
	for _, testcase := range []struct {
		name   string
		limits Limits
	}{
		{"轮数", Limits{Rounds: 1, Tools: 40, Tokens: 80000, Duration: time.Minute, FinalTime: time.Second}},
		{"工具数", Limits{Rounds: 15, Tools: 1, Tokens: 80000, Duration: time.Minute, FinalTime: time.Second}},
		{"token", Limits{Rounds: 15, Tools: 40, Tokens: 14000, Duration: time.Minute, FinalTime: time.Second}},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			queries := 0
			toolbox := toolFunc(func(context.Context, string, json.RawMessage) tools.Result {
				queries++
				return tools.Result{Tool: "query", Status: "ok", Data: json.RawMessage(`null`)}
			})
			model := modelFunc(func(_ context.Context, request llm.Request) (llm.Response, error) {
				response := answer(call(fmt.Sprint(len(request.Messages)), "query", "{}"))
				// token 用例先消耗四千，再检查下一轮是否给最终报告留下空间。
				if testcase.name == "token" {
					response.Usage.Input = 4000
				}
				return response, nil
			})
			agent, _ := New(model, "mock-test", llm.NewRedactor(), testcase.limits)
			result, err := agent.Run(t.Context(), testInput(&memoryRecorder{}, toolbox))
			if err != nil || !result.BudgetExceeded || result.Rounds != 2 || queries != 1 {
				t.Fatalf("result=%+v queries=%d err=%v", result, queries, err)
			}
		})
	}
}

// TestParallelCancel 四个查看同时进入等待；取消后所有查询退出，synctest 会发现残留 goroutine。
func TestParallelCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var active, peak atomic.Int64
		toolbox := toolFunc(func(ctx context.Context, name string, _ json.RawMessage) tools.Result {
			current := active.Add(1)
			for old := peak.Load(); current > old; old = peak.Load() {
				if peak.CompareAndSwap(old, current) {
					break
				}
			}
			defer active.Add(-1)
			<-ctx.Done()
			return tools.Result{Tool: name, Status: "cancelled", Data: json.RawMessage(`null`)}
		})
		model := modelFunc(func(context.Context, llm.Request) (llm.Response, error) {
			return answer(call("1", "query", "{}"), call("2", "query", "{}"), call("3", "query", "{}"), call("4", "query", "{}"), call("5", "query", "{}")), nil
		})
		agent, _ := New(model, "mock-test", llm.NewRedactor(), DefaultLimits())
		ctx, cancel := context.WithCancel(t.Context())
		finished := make(chan error, 1) // 诊断 goroutine 写，主测试在取消后接收并确认退出。
		go func() { _, err := agent.Run(ctx, testInput(&memoryRecorder{}, toolbox)); finished <- err }()
		synctest.Wait()
		if peak.Load() != 4 {
			t.Fatalf("并行数=%d", peak.Load())
		}
		start := time.Now()
		cancel()
		err := <-finished
		if !errors.Is(err, context.Canceled) || active.Load() != 0 || time.Since(start) > 2*time.Second {
			t.Fatalf("取消=%v active=%d", err, active.Load())
		}
	})
}

// TestReportBoundaries 报告字段和建议工具都由代码限制，不依赖提示词是否被模型遵守。
func TestReportBoundaries(t *testing.T) {
	valid := reportArgs(2)
	for _, testcase := range []struct{ name, before, after string }{
		{"组件不在架构", `"component":"unknown"`, `"component":"invented"`},
		{"写工具越界", `"tool":"manual"`, `"tool":"shell"`},
		{"编造步骤", `"step":2`, `"step":99`},
		{"置信度越界", `"confidence":0.1`, `"confidence":2`},
		{"缺必填", `"unknowns":"仍需核对"`, `"extra":"代替必填"`},
		{"人工建议也要风险", `,"risk":"需要人核对"`, ``},
		{"JSON括号未闭合", `"args":{},"risk"`, `"args":{,"risk"`},
		{"重复字段", `"summary":"测试结论"`, `"summary":"第一份","summary":"第二份"`},
	} {
		// 每条用例只改一处，确认拒绝来自对应的结构或业务边界。
		t.Run(testcase.name, func(t *testing.T) {
			if _, err := validateReport([]byte(strings.Replace(valid, testcase.before, testcase.after, 1)), []string{"unknown"}, map[int]bool{2: true}); err == nil {
				t.Fatal("放行错误报告")
			}
		})
	}
	if _, err := validateReport([]byte(valid), []string{"unknown"}, map[int]bool{2: true}); err != nil {
		t.Fatal(err)
	}
}

// TestTimeBudget 使用虚拟时钟走完普通阶段和最后机会，证明总时限不会因整理报告而延长。
func TestTimeBudget(t *testing.T) {
	// 两轮都故意耗尽各自时间；断言真实逻辑只分配五分钟，不把最后机会另加一分钟。
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		model := modelFunc(func(ctx context.Context, request llm.Request) (llm.Response, error) {
			calls++
			if calls == 2 && (len(request.Tools) != 1 || request.Tools[0].Function.Name != "submit_report") {
				t.Fatal("最后机会仍可查询新证据")
			}
			<-ctx.Done()
			return llm.Response{}, ctx.Err()
		})
		agent, _ := New(model, "mock-test", llm.NewRedactor(), DefaultLimits())
		start := time.Now()
		result, err := agent.Run(t.Context(), testInput(&memoryRecorder{}, tools.New(tools.Dependencies{Profile: "架构"})))
		if err != nil || !result.BudgetExceeded || calls != 2 || time.Since(start) != 5*time.Minute {
			t.Fatalf("总时限=%s calls=%d result=%+v err=%v", time.Since(start), calls, result, err)
		}
	})
}

// TestArgumentCorrection 使用真实注册表，证明多传主机参数只会得到错误，不会中止循环。
func TestArgumentCorrection(t *testing.T) {
	// service_topology 没有 host 参数，模型需要通过工具结果知道自己多传了字段。
	calls := 0
	model := modelFunc(func(_ context.Context, request llm.Request) (llm.Response, error) {
		calls++
		if calls == 1 {
			return answer(call("invalid", "service_topology", `{"host":"other.invalid"}`)), nil
		}
		last := request.Messages[len(request.Messages)-1]
		if last.ToolCallID != "invalid" || !strings.Contains(last.Content, "invalid_argument") {
			t.Fatal("参数错误没有按原关联 ID 返回")
		}
		return answer(call("report", "submit_report", reportArgs(0))), nil
	})
	agent, _ := New(model, "mock-test", llm.NewRedactor(), DefaultLimits())
	result, err := agent.Run(t.Context(), testInput(&memoryRecorder{}, tools.New(tools.Dependencies{Profile: "架构"})))
	if err != nil || result.Report == nil || calls != 2 {
		t.Fatalf("参数纠错后未完成：%v", err)
	}
}
