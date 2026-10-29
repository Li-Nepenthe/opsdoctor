// 本文件：按顺序准备医生自己的数据库、接入系统和 HTTP 服务。
//
// 顺序很重要：先校验凭据、完成迁移，再开放告警入口。否则 Alertmanager 可能
// 已经推来通知，医生却还没有表可以保存。启动调度器前还要清理过时和中断的任务。
//
// 主要内容：
//   - Run：启动服务；显式 --clock-samples 则只读本机时钟，不装配任何业务。
//   - serve：等待退出信号，给已经接收的通知有限的提交时间。
//
// 阅读建议：医生自己的数据库与 target 中的只读连接分开创建、分开关闭。

package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/auth"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/httpapi"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/target"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/adminserver"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/clockcheck"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/lifecycle"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/logging"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Run 只有装配完全成功才监听端口，启动失败也会关闭已经创建的资源。
func Run(parent context.Context, args ...string) error {
	// 时钟采样只是容器内读本机时间；必须在读凭据、迁移和启动调度之前分流，
	// 避免每次预检都额外启动一个医生，更不能趁采样触发付费任务。
	if len(args) > 0 {
		if len(args) != 1 || args[0] != "--clock-samples" {
			return fmt.Errorf("医生只接受无参数启动服务，或 --clock-samples 只读采样")
		}
		return clockcheck.Sample(parent, os.Stdout)
	}
	settings, err := loadConfig()
	if err != nil {
		return err
	}
	agentConfig, err := loadAgentConfig(config.New())
	if err != nil {
		return err
	}
	ctx, stop := lifecycle.Notify(parent)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	database, err := mysqlx.Open(startup, mysqlx.Config{DSN: settings.database, MaxOpenConns: 10, MaxIdleConns: 2, ConnMaxLifetime: 5 * time.Minute})
	if err != nil {
		return err
	}
	defer database.Close()
	if err := store.Migrate(startup, database); err != nil {
		return err
	}
	catalog, err := target.Load(settings.targets, os.LookupEnv)
	if err != nil {
		return err
	}
	defer catalog.Close()
	tokens, err := auth.NewTokens(settings.jwtSecret, time.Hour)
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout, "doctor", slog.LevelInfo)
	metrics := telemetry.NewMetrics("doctor")
	repository, scheduler, hub, err := assembleAgent(startup, agentConfig, database, catalog, logger, metrics)
	if err != nil {
		return err
	}
	if err = repository.EnsureUser(startup, settings.email, settings.passwordHash); err != nil {
		return err
	}
	if err = scheduler.Start(ctx); err != nil {
		return err
	}
	defer scheduler.Close()
	mcpHandlers := map[string]http.Handler{}
	for _, name := range catalog.Names() {
		system, _ := catalog.Get(name)
		handler, err := system.MCPHandler(repository, llm.NewRedactor(knownSecrets()...))
		if err != nil {
			return err
		}
		if handler != nil {
			mcpHandlers[name] = handler
		}
	}
	router := httpapi.New(httpapi.Options{Evaluations: repository, Executions: scheduler.ExecutionStatuses, Approvals: repository, MCP: mcpHandlers, Targets: catalog, Store: repository, Tokens: tokens, Logger: logger, Ready: database.PingContext,
		Metrics: promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{}), DummyPasswordHash: settings.passwordHash, Runner: scheduler, Events: hub, Provider: agentConfig.provider, Model: agentConfig.model, SecureCookie: agentConfig.secureCookie})
	// SSE 不能用整个响应十五秒的时限；每帧在处理器中设置五秒写入时限。
	server := &http.Server{Addr: settings.address, Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: time.Minute, MaxHeaderBytes: 16 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
	// 只打印名字与数量，不输出含密码的 DSN、令牌或完整配置。
	logger.Info("医生已启动", "address", settings.address, "targets", catalog.Names(), "version", telemetry.Version, "provider", agentConfig.provider, "model", agentConfig.model, "global_budget_micros", agentConfig.totalMicros, "pending_max_age", agentConfig.pendingAge.String(), "fault_silence_timeout", agentConfig.faultSilence.String())
	return serve(ctx, server)
}

// serve 停止接新请求后等待在途请求；工具最多十秒，停机预算再多留两秒做响应收尾。
func serve(ctx context.Context, server *http.Server) error {
	finished := make(chan error, 1)
	go func() { finished <- server.ListenAndServe() }()
	select {
	case err := <-finished:
		return err
	case <-ctx.Done(): // 信号只打断主流程，已经接收的通知仍有时间提交。
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 12*time.Second)
	defer cancel()
	shutdownErr := adminserver.Shutdown(cleanup, server)
	listenErr := <-finished
	if errors.Is(listenErr, http.ErrServerClosed) {
		listenErr = nil
	}
	return errors.Join(shutdownErr, listenErr)
}
