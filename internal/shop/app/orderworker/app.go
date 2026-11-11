// 本文件：启动订单消费者与管理端口，停机时等当前消息处理完，再释放 Kafka 和数据库连接。

package orderworker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/adminserver"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/lifecycle"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/logging"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/app"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/events"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/migrations"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/sales"
)

// 启动需要连接并检查主题。退出需要在消息预算外再留一点清理时间。
const (
	startupTimeout = 30 * time.Second // 依赖不可用时有界失败，不能永远卡在启动。
	shutdownMargin = 5 * time.Second  // 当前消息完成后，还要关闭管理入口和连接。
)

// Run 先停消费者，再关闭 Kafka 和数据库，不能在当前事务尚未完成时拆掉连接。
func Run(parent context.Context) (result error) {
	settings, err := loadConfig(config.New())
	if err != nil {
		return err
	}
	observability, err := telemetry.New(parent, "order-worker")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, observability.Shutdown(parent)) }()
	settings.Database.TracerProvider = observability.Provider
	logger := logging.New(os.Stdout, observability.Service, slog.LevelInfo)
	ctx, stop := lifecycle.Notify(parent)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	database, err := mysqlx.Open(startup, settings.Database)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, database.Close()) }()
	if err := observability.Metrics.Database(database, "shop"); err != nil {
		return err
	}
	if err := migrations.Apply(startup, database); err != nil {
		return err
	}
	if err := events.EnsureTopic(startup, settings.KafkaBrokers); err != nil {
		return err
	}
	kafkaReader := events.NewReader(settings.KafkaBrokers)
	defer func() { result = errors.Join(result, kafkaReader.Close()) }()
	service := sales.NewService(sales.NewMySQLRepository(database))
	consumer, err := events.NewConsumer(kafkaReader, service.Handle, settings.MessageTimeout, logger, observability)
	if err != nil {
		return err
	}
	admin := adminserver.New(settings.AdminAddress, database.PingContext, observability.Metrics.Registry)
	closeOps, err := app.StartOps(ctx, stop, database, nil, observability.Metrics)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, closeOps()) }()
	cancel()
	// 只输出生效参数，不输出 DSN。消费组名固定，重启才能接着上次的位置读。
	logger.Info("订单消费者配置已加载", "version", telemetry.Version, "admin_addr", settings.AdminAddress,
		"group", "order-worker", "message_timeout", settings.MessageTimeout.String())
	return serve(ctx, consumer, admin, settings.MessageTimeout+shutdownMargin, logger)
}

// serve 在管理端口可用后启动消费，任意循环失败都会取消另一侧并读齐退出结果。
func serve(parent context.Context, consumer *events.Consumer, admin *http.Server, timeout time.Duration, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", admin.Addr)
	if err != nil {
		return fmt.Errorf("监听消费者管理端口：%w", err)
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	finished := make(chan error, 2) // 消费和 HTTP 各返回一次，容量覆盖两者。
	go func() { finished <- consumer.Run(ctx) }()
	go func() { finished <- admin.Serve(listener) }()
	logger.Info("订单消费者已启动")
	remaining := 2
	var result error
	select {
	case <-ctx.Done():
		logger.Info("收到退出信号，等待当前订单消息完成")
	case result = <-finished:
		remaining--
	}
	if errors.Is(result, http.ErrServerClosed) {
		result = nil
	}
	cancel() // FetchMessage 立即结束等待；已取出的消息仍使用自己的处理预算。
	cleanup, stop := context.WithTimeout(context.WithoutCancel(parent), timeout)
	defer stop()
	result = errors.Join(result, adminserver.Shutdown(cleanup, admin))
	for range remaining {
		// 消费者的每条处理都有截止时间，所以这里等待的是有界的当前消息，而不是无限循环。
		err := <-finished
		if !errors.Is(err, http.ErrServerClosed) {
			result = errors.Join(result, err)
		}
	}
	logger.Info("消费循环与管理入口已结束，准备关闭 Kafka 和数据库连接")
	return result
}
