// 本文件：把仓储、模型、Agent、调度和实时通知接成一条执行路径。
//
// HTTP、自动告警和命令行不能各有一份工具实现。这里从接入目录取已经绑定系统的
// 注册表，交给每次诊断使用。存储成功后同时更新指标和事件，页面不提前显示成功。
//
// 主要内容：assembleAgent 装配执行依赖；createModel 只按显式 provider 选择客户端。
// 阅读建议：费用账本总是持久化，Mock 只读额度展示，不向账本预留或扣费。

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

// assembleAgent 只装配，不启动 goroutine；全部成功后 Run 才启动调度和监听端口。
func assembleAgent(ctx context.Context, settings agentSettings, database *sql.DB, catalog *target.Catalog, logger *slog.Logger, common *telemetry.Metrics) (*store.Store, *run.Scheduler, *events.Hub, error) {
	runbooks, err := tools.LoadRunbooks(os.DirFS(settings.runbooks))
	if err != nil {
		return nil, nil, nil, err
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
	metrics, err := newDoctorMetrics(common, toolNames)
	if err != nil {
		return nil, nil, nil, err
	}
	if err = common.Database(database, "doctor"); err != nil {
		return nil, nil, nil, err
	}
	redactor := llm.NewRedactor(append(knownSecrets(), settings.key)...)
	hub := events.New()
	repository := store.NewConfigured(database, store.Config{Currency: settings.currency, LeaseTTL: settings.leaseTTL, ResumeMaxAge: settings.resumeMaxAge, RecoveryWindows: recoveryWindows, FaultSilence: settings.faultSilence, Redactor: redactor, Changed: metrics.changed, Published: func(id string, step agent.Step) {
		// 数据库提交以后只做内存工作，不在这里等待浏览器读取或另发网络请求。
		metrics.step(step)
		raw, _ := json.Marshal(step)
		hub.Publish(id, events.Event{ID: step.Seq, Data: raw})
	}})
	ledger, err := repository.NewCurrencyLedger(ctx, settings.budgetAccount, settings.totalMicros, settings.currency)
	if err != nil {
		return nil, nil, nil, err
	}
	model, err := createModel(settings, ledger, redactor, logger)
	if err != nil {
		return nil, nil, nil, err
	}
	limits := agent.DefaultLimits()
	limits.CostMicros = settings.diagnosisMicros
	loop, err := agent.New(model, settings.model, redactor, limits)
	if err != nil {
		return nil, nil, nil, err
	}
	executions := map[string]run.Execution{"primary": {Model: settings.model, Provider: settings.provider, Account: settings.budgetAccount, Agent: loop, Repository: repository.ForExecution("primary", settings.budgetAccount, settings.currency), Budget: ledger.Status}}
	if settings.comparison != nil {
		// 第二家客户端用同一套脱敏规则，Key 也加入精确替换清单。
		comparisonRedactor := llm.NewRedactor(append(knownSecrets(), settings.key, settings.comparison.key)...)
		execution, err := comparisonExecution(ctx, *settings.comparison, repository, comparisonRedactor, logger)
		if err != nil {
			return nil, nil, nil, err
		}
		executions["comparison"] = execution
	}
	scheduler, err := run.New(run.Config{BudgetAccount: settings.budgetAccount, Executions: executions, LeaseTTL: settings.leaseTTL, Model: settings.model, Provider: settings.provider, Concurrent: settings.concurrent, PerTarget: settings.perTarget, PendingAge: settings.pendingAge, Budget: ledger.Status, ObservedBudget: metrics.budget, Prepare: prepareDiagnosis, Resolve: func(name string) (agent.Input, error) {
		system, ok := catalog.Get(name)
		if !ok {
			return agent.Input{}, fmt.Errorf("任务绑定的系统已不在接入配置中")
		}
		return agent.Input{Profile: system.Profile(), Unavailable: system.Unavailable(), Components: system.Components(), Tools: system.Tools().WithRunbooks(runbooks), Approvals: approval.New(name, system.Actions(), repository, redactor)}, nil
	}}, repository, loop, logger)
	return repository, scheduler, hub, err
}

// createModel 不做探测调用；真实配置不完整时返回错误，不回退成误导用户的脚本报告。
func createModel(settings agentSettings, ledger llm.Ledger, redactor *llm.Redactor, logger *slog.Logger) (llm.Client, error) {
	if settings.provider == "mock" {
		return &mock.Client{}, nil
	}
	return openai.New(openai.Config{Format: settings.apiFormat, BaseURL: settings.baseURL, Key: settings.key, Timeout: settings.timeout, Prices: settings.prices, PricesAt: settings.pricesAt}, nil, ledger, redactor, logger)
}
