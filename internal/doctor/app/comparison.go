// 本文件：配对评测与主模型共用诊断表和工具，使用单独的模型配置和账本。
// loadComparison 先检查提供方和启用开关，再读取第二家的凭据。

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
	// 即使环境中保留了对比配置，Mock 也不能读取真实密钥。
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
	inputs := settingsReader{source: reader, amountError: "%s 必须是正数金额"}
	value.baseURL = inputs.read("EVAL_COMPARISON_BASE_URL", "https://api.deepseek.com")
	value.model = inputs.read("EVAL_COMPARISON_MODEL", "deepseek-flash")
	value.key = inputs.read("DEEPSEEK_API_KEY", "")
	value.budgetAccount = inputs.read("EVAL_COMPARISON_ACCOUNT", "phase-07-08")
	value.totalMicros = inputs.money("EVAL_COMPARISON_TOTAL_CNY", "10")
	value.diagnosisMicros = inputs.money("EVAL_COMPARISON_DIAGNOSIS_CNY", "0.30")
	inputs.readComparisonPrices(&value)
	if value.budgetAccount == primary.budgetAccount || value.model == primary.model || value.diagnosisMicros > value.totalMicros {
		inputs.problems = append(inputs.problems, fmt.Errorf("对比必须用不同模型、独立账户，且单次预算不超过总额"))
	}
	if err = errors.Join(inputs.problems...); err != nil {
		return nil, err
	}
	return &value, nil
}

func (inputs *settingsReader) readComparisonPrices(value *agentSettings) {
	value.prices = llm.Prices{
		InputMicros:    inputs.money("EVAL_COMPARISON_INPUT_PRICE", "2"),
		CacheHitMicros: inputs.money("EVAL_COMPARISON_CACHE_HIT_PRICE", "0.04"),
		OutputMicros:   inputs.money("EVAL_COMPARISON_OUTPUT_PRICE", "8"),
	}
	// 对比固定使用 DeepSeek 官方价表。其他兼容服务不能套用它的闲时折扣。
	if value.baseURL != "https://api.deepseek.com" || value.model != "deepseek-flash" {
		inputs.problems = append(inputs.problems, fmt.Errorf("当前对比仅支持官方 DeepSeek Flash，不能套用其他服务的价格"))
	}
	dates := inputs.read("EVAL_DEEPSEEK_HOLIDAYS", "none")
	var holidays []string
	if dates != "none" {
		holidays = strings.Split(dates, ",")
	}
	tariff, err := llm.DeepSeekTariff(value.prices, holidays)
	value.pricesAt = tariff
	inputs.problems = append(inputs.problems, err)
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
