// 本文件：装配订单服务的依赖、路由和退出流程。
//
// cmd 只调用 Run。本文件先建立连接并迁移，再启动业务与管理端口，退出时先排空请求再关闭连接。
//
// 主要内容：
//   - Run：创建 MySQL、Redis、库存客户端、消息生产者和业务服务。
//   - connectInventory：在开放业务前等待库存连接完成首次握手。
//   - routes：安装 HTTP 保护并登记四个公开接口。
//   - serve：同时管理业务与管理端口，任意一侧异常都会触发停机。
//   - normalHTTPError：只过滤正常关闭的标准错误。
//
// 阅读建议：注意 defer 的后进先出顺序。业务入口关闭以后，底层连接才会被释放。

package orderservice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	inventoryv1 "github.com/Li-Nepenthe/opsdoctor/gen/inventory/v1"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/adminserver"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/httpx"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/lifecycle"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/logging"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/resilience"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/events"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/inventoryclient"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/migrations"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/order"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/product"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// 装配时使用固定的资源边界。业务超时仍由 config.go 中的环境变量决定。
const (
	startupTimeout   = 30 * time.Second // 首次连接与迁移共用预算，依赖不可达时启动应失败。
	redisIOTimeout   = time.Second      // 短 Redis 操作应先于三秒 HTTP 预算结束。
	redisPoolSize    = 20               // 与默认数据库池同量级，避免本机压测无限扩连接。
	headerTimeout    = 3 * time.Second  // 请求头很小，限制慢速发送者占住连接的时间。
	idleTimeout      = 30 * time.Second // 短期复用 HTTP 连接，长期空闲后回收。
	processingMargin = time.Second      // 给时钟偏差和最后的状态写回留出余量。
	cleanupSteps     = 3                // 最慢失败路径依次补偿、删除占位、标记待核对。
	shutdownMargin   = 5 * time.Second  // 在最慢业务路径之外给连接排空留余量。
)

// Run 的 result 是具名返回值，defer 可以把关闭连接的错误一并交给入口。
func Run(parent context.Context) (result error) {
	settings, err := loadConfig(config.New())
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout, "order-service", slog.LevelInfo)
	ctx, stop := lifecycle.Notify(parent)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	database, err := mysqlx.Open(startup, settings.Database)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, database.Close()) }()
	if err := migrations.Apply(startup, database); err != nil {
		return err
	}
	// 禁止 Redis 自动重试：SET NX 响应丢失时，重发可能把“已经占位”误当成另一个请求。
	redisClient := redis.NewClient(&redis.Options{Addr: settings.RedisAddress, MaxRetries: -1, DialTimeout: redisIOTimeout, ReadTimeout: redisIOTimeout, WriteTimeout: redisIOTimeout, ContextTimeoutEnabled: true, PoolSize: redisPoolSize})
	defer func() { result = errors.Join(result, redisClient.Close()) }()
	if err := redisClient.Ping(startup).Err(); err != nil {
		return fmt.Errorf("启动时无法访问 Redis")
	}
	connection, err := grpc.NewClient(settings.InventoryTarget, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry())
	if err != nil {
		return fmt.Errorf("创建库存连接：%w", err)
	}
	defer func() { result = errors.Join(result, connection.Close()) }()
	if err := connectInventory(startup, connection); err != nil {
		return err
	}
	stock, err := inventoryclient.New(inventoryv1.NewInventoryServiceClient(connection), settings.Inventory, logger)
	if err != nil {
		return err
	}
	if err := events.EnsureTopic(startup, settings.KafkaBrokers); err != nil {
		return err
	}
	publisher := events.NewPublisher(settings.KafkaBrokers)
	defer func() { result = errors.Join(result, publisher.Close()) }()
	cache, err := product.NewRedisCache(redisClient, settings.CacheJitter)
	if err != nil {
		return err
	}
	products := product.NewService(product.NewMySQLRepository(database), cache, stock, settings.Singleflight, logger)
	// 最慢路径可能经历补偿、删占位和标记待核对三次收尾。再留一秒时钟与调度余量。
	processingBudget := settings.HTTPBudget + cleanupSteps*settings.CleanupBudget + processingMargin
	idempotency, err := order.NewRedisIdempotency(redisClient, settings.IdempotencyTTL, processingBudget)
	if err != nil {
		return err
	}
	registry := prometheus.NewRegistry()
	reviews, err := newReviewRecorder(logger, registry)
	if err != nil {
		return err
	}
	orders := order.NewService(order.NewMySQLRepository(database), products, stock, publisher, idempotency, reviews, settings.CleanupBudget, logger)
	router, err := routes(settings, products, orders, logger)
	if err != nil {
		return err
	}
	httpServer := &http.Server{Addr: settings.HTTPAddress, Handler: router, ReadHeaderTimeout: headerTimeout, ReadTimeout: settings.HTTPBudget, WriteTimeout: processingBudget + processingMargin, IdleTimeout: idleTimeout}
	admin := adminserver.New(settings.AdminAddress, func(probe context.Context) error {
		if err := database.PingContext(probe); err != nil {
			return err
		}
		return redisClient.Ping(probe).Err()
	})
	// 本阶段只导出用户指定的待核对计数，未注册其他默认运行时或进程指标。
	adminMux := http.NewServeMux()
	adminMux.Handle("/", admin.Handler)
	adminMux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	admin.Handler = adminMux
	cancel()
	logger.Info("订单服务配置已加载", "version", "phase-02", "http_addr", settings.HTTPAddress, "admin_addr", settings.AdminAddress,
		"rate_limit_rps", settings.RateRPS, "rate_limit_burst", settings.RateBurst, "request_timeout", settings.HTTPBudget.String(),
		"inventory_timeout", settings.Inventory.Timeout.String(), "getstock_max_retries", settings.Inventory.Retry.MaxRetries,
		"retry_backoff", settings.Inventory.Retry.Backoff.String(), "breaker_enabled", settings.Inventory.Breaker.Enabled,
		"cache_singleflight", settings.Singleflight, "cache_ttl_jitter", settings.CacheJitter.String(), "idempotency_ttl", settings.IdempotencyTTL.String())
	return serve(ctx, httpServer, admin, processingBudget+shutdownMargin, logger)
}

// connectInventory 把首次 DNS 查询和握手放进启动预算，避免第一位用户替服务承担建连等待。
// NewClient 默认只创建一个空闲连接管理器。Connect 触发建连，Ready 表示传输已经可以接收 RPC。
func connectInventory(ctx context.Context, connection *grpc.ClientConn) error {
	connection.Connect()
	for {
		state := connection.GetState()
		if state == connectivity.Ready {
			return nil
		}
		if state == connectivity.Shutdown {
			return fmt.Errorf("库存连接在启动时已关闭")
		}
		// 状态变化也可能只是从 Connecting 变成 TransientFailure，醒来后必须重新检查。
		if !connection.WaitForStateChange(ctx, state) {
			return fmt.Errorf("等待库存连接就绪：%w", ctx.Err())
		}
	}
}

// routes 先安装请求标识和恢复，再限流、限制正文与时间，最后才进入业务。
func routes(settings Config, products *product.Service, orders *order.Service, logger *slog.Logger) (*gin.Engine, error) {
	limit, err := resilience.RateLimit(settings.RateRPS, settings.RateBurst)
	if err != nil {
		return nil, err
	}
	router := gin.New()
	router.Use(httpx.RequestIDs(logger), httpx.Recovery(logger), limit, httpx.BodyLimit(httpx.DefaultBodyLimit), resilience.RequestBudget(settings.HTTPBudget))
	productHandler, orderHandler := product.NewHandler(products), order.NewHandler(orders)
	router.GET("/products/:id", productHandler.Get)
	router.POST("/orders", orderHandler.Create)
	router.GET("/orders/:id", orderHandler.Get)
	router.GET("/orders", orderHandler.List)
	router.NoRoute(func(ctx *gin.Context) { httpx.Failure(ctx, http.StatusNotFound, "INVALID_ARGUMENT", "接口不存在") })
	return router, nil
}

// serve 等两个端口都绑定成功才开始接流量，避免只启动了半个进程。
func serve(ctx context.Context, business, admin *http.Server, timeout time.Duration, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", business.Addr)
	if err != nil {
		return fmt.Errorf("监听订单 HTTP：%w", err)
	}
	adminListener, err := net.Listen("tcp", admin.Addr)
	if err != nil {
		return errors.Join(fmt.Errorf("监听订单管理端口：%w", err), listener.Close())
	}
	finished := make(chan error, 2) // 两个 Serve 各写一次，主流程会读齐。
	go func() { finished <- business.Serve(listener) }()
	go func() { finished <- admin.Serve(adminListener) }()
	logger.Info("订单服务已启动")
	remaining := 2
	var serveErr error
	select {
	case <-ctx.Done():
		logger.Info("收到退出信号，开始排空订单请求")
	case serveErr = <-finished:
		remaining--
	}
	serveErr = normalHTTPError(serveErr)
	// 先排空业务，再关管理端口。Run 的 defer 最后才关闭连接和生产者。
	closeErr := lifecycle.Shutdown(ctx, timeout,
		lifecycle.Hook{Name: "订单 HTTP", Close: func(cleanup context.Context) error { return adminserver.Shutdown(cleanup, business) }},
		lifecycle.Hook{Name: "订单管理端口", Close: func(cleanup context.Context) error { return adminserver.Shutdown(cleanup, admin) }})
	for range remaining {
		serveErr = errors.Join(serveErr, normalHTTPError(<-finished))
	}
	logger.Info("订单入口已关闭，准备关闭下游连接", "drain_failed", closeErr != nil)
	return errors.Join(serveErr, closeErr)
}

// normalHTTPError 只忽略标准的正常关闭错误，绑定后的监听故障仍交给入口返回失败码。
func normalHTTPError(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
