// 本文件：把评委接入现有预算账本，并保存可续跑的评分结果。
//
// 评委和 DeepSeek 对比共用人民币账户。它不借用 Terra 诊断的美元归属，也不
// 绕过发送前预留。已有 running 评分只从审计恢复，不再发第二轮请求挑分数。
//
// 主要内容：newGrader 装配 Mock 或收费客户端；gradeSlot 保存评分输入、费用和结果；
// gradeInput 只把医生已保存的观察交给评委。
// 阅读建议：外层持有批次锁；同一个槽位的评分一旦保存，就原样返回。

package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm/openai"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/eval/judge"
	"github.com/Li-Nepenthe/opsdoctor/internal/eval/scoring"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// grader 不含靶场客户端，唯一的外部模型入口已经绑定固定账户。
type grader struct {
	client                   llm.Client   // Mock 或原有收费客户端。
	model, provider, account string       // 公开配置，用于审计比对。
	repository               *store.Store // 与账户同为 CNY 的仓储视图。
}

// gradedResult 失败也保留；coverage 只在评分通过校验时有效。
type gradedResult struct {
	Version    string                   `json:"version"`         // 评分标准版本。
	Hash       string                   `json:"hash"`            // 实际标准的字节哈希。
	Status     string                   `json:"status"`          // ok 或 failed。
	Model      string                   `json:"model"`           // 评委实际使用模型。
	Error      string                   `json:"error,omitempty"` // 评分失败的原因。
	Grade      judge.Grade              `json:"grade"`           // 机制和非规则证据评分。
	Evidence   []scoring.EvidenceResult `json:"evidence"`        // 包含规则与评委的全部项目。
	Coverage   float64                  `json:"coverage"`        // achieved / 全部项目数。
	Usage      llm.Usage                `json:"usage"`           // 此次已确认用量。
	CostMicros int64                    `json:"cost_micros"`     // 包含网络失败预留，币种固定 CNY。
	Currency   string                   `json:"currency"`        // 不与医生的 USD 相加。
}

// newGrader 同时要求运行中的医生与本机显式允许真实调用，填 Key 本身不会启用收费。
func newGrader(ctx context.Context, reader config.Reader, repository *store.Store, status runtimeStatus, paid bool, output io.Writer) (*grader, error) {
	value := &grader{client: judge.Mock{}, model: "mock-judge-v1", provider: "mock", account: "phase-07-08", repository: repository.ForExecution("judge", "phase-07-08", "CNY")}
	if status.Provider == "mock" {
		return value, nil
	}
	provider, err := reader.String("LLM_PROVIDER", "mock")
	if err != nil || provider != "openai" || !paid {
		return nil, fmt.Errorf("真实评委需要 LLM_PROVIDER=openai 和 --allow-paid")
	}
	profile, ok := status.Executions["comparison"]
	if !ok || profile.Model != "deepseek-flash" || profile.Account != "phase-07-08" || profile.Budget.Currency != "CNY" || profile.Budget.LimitMicros > 10000000 {
		return nil, fmt.Errorf("运行中的 DeepSeek 配对账户不符合已授权范围")
	}
	key, err := reader.String("DEEPSEEK_API_KEY", "")
	if err != nil {
		return nil, err
	}
	prices := llm.Prices{}
	for _, item := range []struct {
		name, fallback string
		target         *int64
	}{
		{"EVAL_COMPARISON_INPUT_PRICE", "2", &prices.InputMicros}, // 与对比模型使用同一份峰时价。
		{"EVAL_COMPARISON_CACHE_HIT_PRICE", "0.04", &prices.CacheHitMicros},
		{"EVAL_COMPARISON_OUTPUT_PRICE", "8", &prices.OutputMicros},
	} {
		text, err := reader.String(item.name, item.fallback)
		if err != nil {
			return nil, err
		}
		*item.target, err = llm.Money(text)
		if err != nil {
			return nil, err
		}
	}
	dates, err := reader.String("EVAL_DEEPSEEK_HOLIDAYS", "none")
	if err != nil {
		return nil, err
	}
	var holidays []string
	if dates != "none" {
		holidays = strings.Split(dates, ",")
	}
	pricesAt, err := llm.DeepSeekTariff(prices, holidays)
	if err != nil {
		return nil, err
	}
	// 模式、地址和账号在这里固定；报告里的文字不能改变下一次请求的目的地。
	ledger, err := value.repository.NewCurrencyLedger(ctx, value.account, profile.Budget.LimitMicros, "CNY")
	if err != nil {
		return nil, err
	}
	secrets := []string{key}
	for _, entry := range os.Environ() {
		name, text, _ := strings.Cut(entry, "=")
		if strings.HasSuffix(name, "_API_KEY") || strings.HasSuffix(name, "_PASSWORD") || strings.HasSuffix(name, "_TOKEN") {
			secrets = append(secrets, text)
		}
	}
	value.client, err = openai.New(openai.Config{BaseURL: "https://api.deepseek.com", Key: key, Timeout: time.Minute, Prices: prices, PricesAt: pricesAt}, nil, ledger, llm.NewRedactor(secrets...), slog.New(slog.NewTextHandler(output, nil)))
	value.model, value.provider = "deepseek-flash", "openai"
	return value, err
}

// gradeInput 保留所有实际观察，不把医生的思考文本或模型品牌发给评委。
func gradeInput(detail store.Detail, value scenario.Scenario) judge.Input {
	input := judge.Input{Truth: value.GroundTruth, Report: detail.Report, Symptoms: detail.Input, Steps: judge.Observations(detail.Steps), Milestones: value.Evidence, Rules: []scoring.EvidenceResult{}}
	for _, rule := range value.Rules {
		input.Rules = append(input.Rules, scoring.Evidence(scoring.Rule{Milestone: rule.Milestone, Kind: rule.Kind, Table: rule.Table, Index: rule.Index}, detail.Steps))
	}
	return input
}

// gradeSlot 对恢复的回复也重新做引用校验，但不会再次付费；原始失败仍留在医生记录里。
func (value *grader) gradeSlot(ctx context.Context, slot store.EvalResult, scenario scenario.Scenario) error {
	if len(slot.Judgement) > 0 {
		return nil
	}
	detail, err := value.repository.Detail(ctx, slot.DiagnosisID)
	if err != nil {
		return err
	}
	if len(detail.Report) == 0 {
		return nil
	} // 没有报告无法评机制，保留原失败，不为一份空报告付钱。
	input := gradeInput(detail, scenario)
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	audit, err := value.repository.PrepareEvalJudge(ctx, slot.Key, value.model, value.provider, value.account, judge.RubricVersion, raw)
	if err != nil {
		return err
	}
	// 使用实际落库的脱敏输入，审计内容与发送内容一致。
	if err = json.Unmarshal(audit.Input, &input); err != nil {
		return err
	}
	var result judge.Grade
	var response llm.Response
	var problem error
	var elapsed *time.Duration
	if audit.Status == "pending" {
		started, err := value.repository.StartEvalJudge(ctx, audit.ID)
		if err != nil || !started {
			return fmt.Errorf("评分未取得执行权：%v", err)
		}
		begin := time.Now()
		result, response, problem = judge.Evaluate(ctx, value.client, value.model, audit.ID, input)
		duration := time.Since(begin)
		elapsed = &duration
	} else if audit.Status == "running" {
		response, err = value.repository.SavedEvalJudgeResponse(ctx, audit.ID)
		if err != nil {
			return err
		}
		result, problem = judge.CheckResponse(response, input)
	} else {
		return fmt.Errorf("评分已结束但缺少结果，需要核对审计，不能重新付费")
	}
	grade := gradedResult{Version: judge.RubricVersion, Hash: judge.RubricHash(), Status: "ok", Model: response.Model, Grade: result, Usage: response.Usage, CostMicros: response.CostMicros, Currency: "CNY", Evidence: []scoring.EvidenceResult{}}
	if problem != nil {
		grade.Status, grade.Error = "failed", problem.Error()
	} else {
		grade.Evidence = append(grade.Evidence, input.Rules...)
		grade.Evidence = append(grade.Evidence, result.Milestones...)
		grade.Coverage = scoring.Coverage(grade.Evidence)
	}
	encoded, _ := json.Marshal(grade)
	// 取消只中断付费请求，收尾仍留短预算保存已发生的费用与失败。
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err = value.repository.FinishEvalJudge(cleanup, slot.Key, audit.ID, encoded, response, elapsed, grade.Error); err != nil {
		return err
	}
	if errors.Is(problem, llm.ErrBudget) || errors.Is(problem, llm.ErrEvaluationBudget) {
		return problem
	}
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	return nil
}
