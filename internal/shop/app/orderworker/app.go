// 本文件：启动订单消息消费者，并管理它的停机顺序。
//
// cmd 把控制权交给 Run。这里建立数据库和 Kafka 连接，销量事务由 sales 包完成。
//
// 主要内容：
//   - Run：读取配置、应用迁移，创建消息读取器与销量服务。
//   - serve：同时运行消费循环和管理端口，收到停止信号后等待当前消息。
//
// 阅读建议：消费循环拿到的是可取消 context，手中那条消息的收尾预算则由 events.Consumer 提供。

package orderworker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/adminserver"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/lifecycle"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/logging"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
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
	reader := config.New()
	databaseConfig, databaseErr := app.DatabaseConfig(reader)
	address, addressErr := reader.String("ADMIN_ADDR", ":6063")
	rawBrokers, brokersErr := reader.String("KAFKA_BROKERS", "127.0.0.1:9092")
	messageTimeout, timeoutErr := reader.Duration("WORKER_MESSAGE_TIMEOUT", 10*time.Second, time.Second, 30*time.Second)
	if err := errors.Join(databaseErr, addressErr, brokersErr, timeoutErr); err != nil {
		return err
	}
	brokers := strings.Split(rawBrokers, ",")
	for index, broker := range brokers {
		broker = strings.TrimSpace(broker)
		if host, port, err := net.SplitHostPort(broker); err != nil || host == "" || port == "" {
			return fmt.Errorf("KAFKA_BROKERS 必须是逗号分隔的主机:端口")
		}
		brokers[index] = broker
	}
	logger := logging.New(os.Stdout, "order-worker", slog.LevelInfo)
	ctx, stop := lifecycle.Notify(parent)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	database, err := mysqlx.Open(startup, databaseConfig)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, database.Close()) }()
	if err := migrations.Apply(startup, database); err != nil {
		return err
	}
	if err := events.EnsureTopic(startup, brokers); err != nil {
		return err
	}
	kafkaReader := events.NewReader(brokers)
	defer func() { result = errors.Join(result, kafkaReader.Close()) }()
	service := sales.NewService(sales.NewMySQLRepository(database))
	consumer, err := events.NewConsumer(kafkaReader, service.Handle, messageTimeout, logger)
	if err != nil {
		return err
	}
	admin := adminserver.New(address, database.PingContext)
	cancel()
	// 只输出生效参数，不输出 DSN。消费组名固定，重启才能接着上次的位置读。
	logger.Info("订单消费者配置已加载", "version", "phase-02", "admin_addr", address, "group", "order-worker", "message_timeout", messageTimeout.String())
	return serve(ctx, consumer, admin, messageTimeout+shutdownMargin, logger)
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
