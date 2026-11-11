// 本文件：assembleAgent 为各种诊断入口装配同一套仓储、工具、额度和调度规则。
// 步骤提交后才更新指标并通知订阅者。

package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/approval"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/events"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm/mock"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm/openai"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/target"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
)

type agentDependencies struct {
	startup  context.Context
	settings agentSettings
	database *sql.DB
	catalog  *target.Catalog
	logger   *slog.Logger
	metrics  *telemetry.Metrics
}

type agentRuntime struct {
	repository *store.Store
	scheduler  *run.Scheduler
	hub        *events.Hub
}

// assembleAgent 在调度开始之前装配依赖，装配阶段不发模型请求。
func assembleAgent(input agentDependencies) (agentRuntime, error) {
	settings := input.settings
	catalog := input.catalog
	runbooks, err := tools.LoadRunbooks(os.DirFS(settings.runbooks))
	if err != nil {
		return agentRuntime{}, err
	}
	toolNames := map[string]bool{}
	toolNames["runbook_search"] = true
	recoveryWindows := map[string]time.Duration{}
	for _, name := range catalog.Names() {
		system, _ := catalog.Get(name)
		recoveryWindows[name] = system.RecoveryWindow()
		for _, definition := range system.Tools().List() {
			toolNames[definition.Name] = true
		}
	}
	for _, name := range []string{"create_index", "set_db_pool_size", "warm_cache"} {
		toolNames[name] = true
	}
	metrics, err := newDoctorMetrics(input.metrics, toolNames)
	if err != nil {
		return agentRuntime{}, err
	}
	if err = input.metrics.Database(input.database, "doctor"); err != nil {
		return agentRuntime{}, err
	}
	redactor := llm.NewRedactor(append(knownSecrets(), settings.key)...)
	hub := events.New()
	repository := store.NewConfigured(input.database, store.Config{Currency: settings.currency, LeaseTTL: settings.leaseTTL, ResumeMaxAge: settings.resumeMaxAge, RecoveryWindows: recoveryWindows, FaultSilence: settings.faultSilence, Redactor: redactor, Changed: metrics.changed, Published: func(id string, step agent.Step) {
		// 数据库提交以后只做内存工作，不在这里等待浏览器读取或另发网络请求。
		metrics.step(step)
		raw, _ := json.Marshal(step)
		hub.Publish(id, events.Event{ID: step.Seq, Data: raw})
	}})
	ledger, err := repository.NewCurrencyLedger(input.startup, settings.budgetAccount, settings.totalMicros, settings.currency)
	if err != nil {
		return agentRuntime{}, err
	}
	model, err := createModel(settings, ledger, redactor, input.logger)
	if err != nil {
		return agentRuntime{}, err
	}
	limits := agent.DefaultLimits()
	limits.CostMicros = settings.diagnosisMicros
	loop, err := agent.New(model, settings.model, redactor, limits)
	if err != nil {
		return agentRuntime{}, err
	}
	executions := map[string]run.Execution{"primary": {Model: settings.model, Provider: settings.provider, Account: settings.budgetAccount, Agent: loop, Repository: repository.ForExecution("primary", settings.budgetAccount, settings.currency), Budget: ledger.Status}}
	if settings.comparison != nil {
		// 第二家客户端用同一套脱敏规则，Key 也加入精确替换清单。
		comparisonRedactor := llm.NewRedactor(append(knownSecrets(), settings.key, settings.comparison.key)...)
		execution, err := comparisonExecution(input.startup, *settings.comparison, repository, comparisonRedactor, input.logger)
		if err != nil {
			return agentRuntime{}, err
		}
		executions["comparison"] = execution
	}
	scheduler, err := run.New(run.Config{BudgetAccount: settings.budgetAccount, Executions: executions, LeaseTTL: settings.leaseTTL, Model: settings.model, Provider: settings.provider, Concurrent: settings.concurrent, PerTarget: settings.perTarget, PendingAge: settings.pendingAge, Budget: ledger.Status, ObservedBudget: metrics.budget, Prepare: prepareDiagnosis, Resolve: func(name string) (agent.Input, error) {
		system, ok := catalog.Get(name)
		if !ok {
			return agent.Input{}, fmt.Errorf("任务绑定的系统已不在接入配置中")
		}
		return agent.Input{Profile: system.Profile(), Unavailable: system.Unavailable(), Components: system.Components(), Tools: system.Tools().WithRunbooks(runbooks), Approvals: approval.New(name, system.Actions(), repository, redactor)}, nil
	}}, repository, loop, input.logger)
	return agentRuntime{repository: repository, scheduler: scheduler, hub: hub}, err
}

// createModel 不做探测调用；真实配置不完整时返回错误，不回退成误导用户的脚本报告。
func createModel(settings agentSettings, ledger llm.Ledger, redactor *llm.Redactor, logger *slog.Logger) (llm.Client, error) {
	if settings.provider == "mock" {
		return &mock.Client{}, nil
	}
	return openai.New(openai.Config{Format: settings.apiFormat, BaseURL: settings.baseURL, Key: settings.key, Timeout: settings.timeout, Prices: settings.prices, PricesAt: settings.pricesAt}, nil, ledger, redactor, logger)
}
