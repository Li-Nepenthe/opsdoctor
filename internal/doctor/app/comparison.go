// 本文件：在真实评测时额外装配第二家模型，不切换正在运行的全局客户端。
//
// 正常启动仍然只用 LLM_PROVIDER 指定的模型。只有显式启用对比，才读取第二家的
// Key；Mock 模式无论填了什么都不装配真实对比客户端。
//
// 主要内容：loadComparison 读取第二套固定配置；comparisonExecution 创建独立账本和 Agent。
// 阅读建议：二者共用诊断表与工具，只有模型请求路径、单价和账户不同。

package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// loadComparison 只在真实模式读额外凭据，避免集成测试中的告警意外触发收费。
func loadComparison(reader config.Reader, primary agentSettings) (*agentSettings, error) {
	// Mock 是最外层开关。即使旧 .env 留着 enabled=true，也不能读取或启用真实 Key。
	if primary.provider == "mock" {
		return nil, nil
	}
	enabled, err := reader.Bool("EVAL_COMPARISON_ENABLED", false)
	if err != nil || !enabled {
		return nil, err
	}
	value := primary
	value.comparison = nil
	value.currency, value.apiFormat = "CNY", "chat_completions"
	var problems []error
	read := func(name, fallback string) string {
		text, err := reader.String(name, fallback)
		problems = append(problems, err)
		return text
	}
	money := func(name, fallback string) int64 {
		amount, err := llm.Money(read(name, fallback))
		if err != nil || amount <= 0 {
			problems = append(problems, fmt.Errorf("%s 必须是正数金额", name))
		}
		return amount
	}
	value.baseURL = read("EVAL_COMPARISON_BASE_URL", "https://api.deepseek.com")
	value.model = read("EVAL_COMPARISON_MODEL", "deepseek-flash")
	value.key = read("DEEPSEEK_API_KEY", "")
	value.budgetAccount = read("EVAL_COMPARISON_ACCOUNT", "phase-07-08")
	value.totalMicros = money("EVAL_COMPARISON_TOTAL_CNY", "10")
	value.diagnosisMicros = money("EVAL_COMPARISON_DIAGNOSIS_CNY", "0.30")
	value.prices = llm.Prices{InputMicros: money("EVAL_COMPARISON_INPUT_PRICE", "2"), CacheHitMicros: money("EVAL_COMPARISON_CACHE_HIT_PRICE", "0.04"), OutputMicros: money("EVAL_COMPARISON_OUTPUT_PRICE", "8")}
	// 对比固定使用 DeepSeek 官方价表。其他兼容服务不能套用它的闲时折扣。
	if value.baseURL != "https://api.deepseek.com" || value.model != "deepseek-flash" {
		problems = append(problems, fmt.Errorf("当前对比仅支持官方 DeepSeek Flash，不能套用其他服务的价格"))
	}
	dates := read("EVAL_DEEPSEEK_HOLIDAYS", "none")
	var holidays []string
	if dates != "none" {
		holidays = strings.Split(dates, ",")
	}
	value.pricesAt, err = llm.DeepSeekTariff(value.prices, holidays)
	problems = append(problems, err)
	if value.budgetAccount == primary.budgetAccount || value.model == primary.model || value.diagnosisMicros > value.totalMicros {
		problems = append(problems, fmt.Errorf("对比必须用不同模型、独立账户，且单次预算不超过总额"))
	}
	if err = errors.Join(problems...); err != nil {
		return nil, err
	}
	return &value, nil
}

// comparisonExecution 仍用同一个 OpenAI 协议适配器，费用没有另外开一条免审计路径。
func comparisonExecution(ctx context.Context, settings agentSettings, repository *store.Store, redactor *llm.Redactor, logger *slog.Logger) (run.Execution, error) {
	// 仓储视图不增加数据库连接，只改变领租约与记步骤时核对的账户和币种。
	bound := repository.ForExecution("comparison", settings.budgetAccount, settings.currency)
	ledger, err := bound.NewCurrencyLedger(ctx, settings.budgetAccount, settings.totalMicros, settings.currency)
	if err != nil {
		return run.Execution{}, err
	}
	model, err := createModel(settings, ledger, redactor, logger)
	if err != nil {
		return run.Execution{}, err
	}
	limits := agent.DefaultLimits()
	limits.CostMicros = settings.diagnosisMicros
	loop, err := agent.New(model, settings.model, redactor, limits)
	if err != nil {
		return run.Execution{}, err
	}
	return run.Execution{Model: settings.model, Provider: settings.provider, Account: settings.budgetAccount, Agent: loop, Repository: bound, Budget: ledger.Status}, nil
}
