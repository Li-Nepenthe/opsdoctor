// 本文件：只用Mock检查阶段9的评分失败边界，不启动数据库、Docker或真实模型。
//
// 已取得回答但冻结校验拒收，保持评分缺失；请求失败、未知计量和本批第三份失败仍停。
// 另检查只有primary的Mock配置与五分钟等待，原阶段8评分字段保持不变。

package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/eval/judge"
	"github.com/Li-Nepenthe/opsdoctor/internal/eval/report"
)

// demoJudgeMock只返回给定外层响应；calls用于证明已存失败不会重新请求。
type demoJudgeMock struct {
	response llm.Response
	err      error
	calls    int
}

// Complete不读取配置或Key，失败时也保留给定的原响应。
func (mock *demoJudgeMock) Complete(context.Context, llm.Request) (llm.Response, error) {
	mock.calls++
	return mock.response, mock.err
}

// TestGeneralizationJudgeReturn区分完成后的格式错误与API/预算/未知返回，不靠错误文本。
func TestGeneralizationJudgeReturn(t *testing.T) {
	response := llm.Response{Model: "deepseek-flash", UsageKnown: true, AttemptIDs: []string{"settled-attempt"}, CostMicros: 12,
		Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{Function: llm.Function{Name: "submit_grade", Arguments: `{"reason":"未闭合`}}}}}
	input := judge.Input{Report: json.RawMessage(`{}`), Milestones: []string{"真实项目"}}
	// 外层有模型、计量与账本归属就是已返回；调用数量和内部字段交给冻结Check拒收。
	empty := response
	empty.Message = llm.Message{Role: "assistant"}
	twice := response
	twice.Message.ToolCalls = []llm.ToolCall{response.Message.ToolCalls[0], response.Message.ToolCalls[0]}
	arguments := func(raw string) llm.Response {
		value := response
		value.Message.ToolCalls = []llm.ToolCall{{Function: llm.Function{Name: "submit_grade", Arguments: raw}}}
		return value
	}
	for _, test := range []struct {
		name     string
		response llm.Response
		err      error
		returned bool
	}{
		{"已返回但JSON不合法", response, nil, true},
		{"已返回空assistant零次调用", empty, nil, true},
		{"已返回两次调用", twice, nil, true},
		{"已返回但含未知字段", arguments(`{"mechanism":"partial","reason":"已返回","unrelated_action":false,"action_reason":"无写操作","milestones":[],"extra":true}`), nil, true},
		{"已返回但编造步骤", arguments(`{"mechanism":"partial","reason":"已返回","unrelated_action":false,"action_reason":"无写操作","milestones":[{"milestone":1,"achieved":true,"steps":[999],"reason":"引用不存在步骤"}]}`), nil, true},
		{"API失败恰好也写JSON错误", response, errors.New("评分 JSON 格式不合法"), false},
		{"有限重试后仍失败", response, errors.New("network unavailable"), false},
		{"账户预算不足", response, llm.ErrBudget, false},
		{"批次token不足", response, llm.ErrEvaluationBudget, false},
		{"取消后有部分响应", response, context.Canceled, false},
		{"无响应却返回nil", llm.Response{}, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			mock := &demoJudgeMock{response: test.response, err: test.err}
			observed := &demoJudgeClient{Client: mock}
			_, saved, err := judge.Evaluate(t.Context(), observed, "deepseek-flash", "judge", input)
			if err == nil || observed.returned != test.returned || mock.calls != 1 || saved.CostMicros != test.response.CostMicros {
				t.Fatal("返回证明丢失API边界或原费用", observed.returned, saved, err)
			}
			if test.returned {
				data, _ := json.Marshal(report.Summary{Status: "succeeded"})
				grade, _ := json.Marshal(gradedResult{Status: "failed", ResponseReturned: &observed.returned})
				row := store.EvalResult{RunID: "phase09-protocol", Key: "slot", JudgeID: "judge", Data: data, Judgement: grade}
				if checkDemoJudgements(row.RunID, row.Key, []store.EvalResult{row}) != nil {
					t.Fatal("冻结校验拒收的首次评分失败被误当API失败")
				}
			}
		})
	}
	for _, change := range []func(*llm.Response){
		func(value *llm.Response) { value.UsageKnown = false },
		func(value *llm.Response) { value.AttemptIDs = nil },
		func(value *llm.Response) { value.Message = llm.Message{} },
	} {
		unknown := response
		change(&unknown)
		if demoJudgeResponse(unknown, false) {
			t.Fatal("未知计量或无可核对返回被放行")
		}
	}
	// 原客户端已完成自己的有限重试；最终成功不会仅因存在多个attempt而被拒绝。
	response.AttemptIDs = []string{"earlier-attempt", "settled-final"}
	if !demoJudgeResponse(response, false) {
		t.Fatal("最终成功被错误地当成API重试失败")
	}
	if demoJudgeResponse(llm.Response{}, true) {
		t.Fatal("Mock也不能把无响应伪装成返回")
	}
}

// TestGeneralizationJudgeFailures前两份格式失败不重评并允许恢复，第三份按本run累计硬停。
func TestGeneralizationJudgeFailures(t *testing.T) {
	runID := "phase09-new-formal"
	confirmed := true
	data, _ := json.Marshal(report.Summary{Status: "succeeded", DataQuality: report.QualityNormal})
	grade, _ := json.Marshal(gradedResult{Status: "failed", Error: "任意冻结校验错误", ResponseReturned: &confirmed})
	rows := []store.EvalResult{{RunID: "phase09-pilot2-20261103-terra", Key: "old", Data: data, Judgement: json.RawMessage(`{"status":"failed"}`)}}
	for index := 1; index <= 3; index++ {
		key := fmt.Sprintf("%s/OT01/%d", runID, index)
		row := store.EvalResult{RunID: runID, Key: key, JudgeID: fmt.Sprintf("judge%d", index), Data: data, Judgement: grade}
		rows = append(rows, row)
		err := checkDemoJudgements(runID, key, rows)
		if (err == nil) != (index < 3) {
			t.Fatalf("本批第%d份失败停点错误：%v", index, err)
		}
		if index == 3 && !strings.Contains(err.Error(), "累计3份") {
			t.Fatal(err)
		}
		if index < 3 {
			// gradeSlot首先复用已存judgement，不访问repository，也不能重问直到通过。
			mock := &demoJudgeMock{err: errors.New("不应重问")}
			grader := grader{client: mock, account: "phase-09-deepseek"}
			if err = grader.gradeSlot(t.Context(), row, scenario.Scenario{}); err != nil || mock.calls != 0 || string(row.Judgement) != string(grade) {
				t.Fatal("已存失败被覆盖或再次评分", err, mock.calls)
			}
			if checkDemoJudgements(runID, "", rows) != nil {
				t.Fatal("前两份失败阻止了本轮恢复与冷却")
			}
		}
	}
	if checkDemoJudgements(runID, "", rows) == nil {
		t.Fatal("续跑绕过第三份失败停点")
	}
	for _, proof := range []*bool{nil, new(bool)} {
		failed, _ := json.Marshal(gradedResult{Status: "failed", Error: "不是用文字决定类别", ResponseReturned: proof})
		if checkDemoJudgements(runID, "slot", []store.EvalResult{{RunID: runID, Key: "slot", JudgeID: "judge", Data: data, Judgement: failed}}) == nil {
			t.Fatal("API失败或旧评分无返回证明仍被放行")
		}
		validButUnknown, _ := json.Marshal(gradedResult{Status: "ok", ResponseReturned: proof})
		if checkDemoJudgements(runID, "slot", []store.EvalResult{{RunID: runID, Key: "slot", JudgeID: "judge", Data: data, Judgement: validButUnknown}}) == nil {
			t.Fatal("评分通过不能掩盖费用或返回证明未知")
		}
	}
	if checkDemoJudgements(runID, "missing", nil) == nil {
		t.Fatal("没有原槽位也当作评分完成")
	}
	legacy, _ := json.Marshal(gradedResult{Status: "ok"})
	if strings.Contains(string(legacy), "response_returned") {
		t.Fatal("阶段8旧序列化被增加阶段9字段")
	}
}

// TestGeneralizationMockPrimary允许免费医生只暴露primary，真实账户仍需严格双配置。
func TestGeneralizationMockPrimary(t *testing.T) {
	// 免费运行只暴露医生配置，不能要求不存在的真实对比客户端。
	status := runtimeStatus{Provider: "mock", Executions: map[string]run.ExecutionStatus{"primary": {Provider: "mock", Model: "mock-v1", Account: "phase-09-openai", Budget: run.BudgetStatus{Currency: "USD", LimitMicros: 10000000}}}}
	if values, err := generalizationConfigurations(status, "mock-v1", []string{"E1", "E2"}); err != nil || len(values) != 2 || values[0].Provider != "mock" {
		t.Fatal(values, err)
	}
	status.Provider = "openai"
	primary := status.Executions["primary"]
	primary.Provider = "openai"
	primary.Model = "gpt-6.1-terra"
	status.Executions["primary"] = primary
	if _, err := generalizationConfigurations(status, "gpt-6.1-terra", []string{"E1"}); err == nil {
		t.Fatal("真实模型借Mock兼容跳过DeepSeek账户")
	}
}

// TestGeneralizationObservationLimit验证五分钟边界和首次通过回调，保存失败会停止推进。
func TestGeneralizationObservationLimit(t *testing.T) {
	// 时限放宽只改变允许范围；非正数和超过五分钟仍然拒绝。
	for _, within := range []string{"3m", "5m", "5m1ns", "0s"} {
		raw := []byte(fmt.Sprintf(`{"id":"OT01","title":"本地测试","source_url":"https://opentelemetry.io/","flag":"adFailure","variant":"on","symptoms":"观察错误","symptom":{"promql":"x > 0","within":%q},"healthy_promql":"x == 0","ground_truth":{"component":"ad","mechanism":"参考机制","explanation":"参考说明"},"evidence_milestones":["证据"]}`, within))
		_, err := decodeDemoScenario(raw)
		if (err == nil) != (within == "3m" || within == "5m") {
			t.Fatal(within, err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"status":"success","data":{"result":[{"value":[1,"0"]}]}}`)
	}))
	defer server.Close()
	called := 0
	failure := errors.New("独占计时证据保存失败")
	err := waitDemoQueryObserved(t.Context(), server.URL, "x == 0", time.Now().Add(demoObservationLimit), func(at time.Time, polls int) error {
		called++
		if at.IsZero() || polls != 1 {
			t.Error("首次观测时刻或轮询次数错误")
		}
		return failure
	})
	if !errors.Is(err, failure) || called != 1 {
		t.Fatal("计时原件失败仍推进恢复", err, called)
	}
	if err = waitDemoQuery(t.Context(), server.URL, "x == 0", time.Now().Add(time.Minute)); err != nil {
		t.Fatal("未提供观察回调时改变了原等待结果", err)
	}
}
