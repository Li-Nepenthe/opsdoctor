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

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/adminserver"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/httpx"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/lifecycle"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/logging"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/resilience"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/order"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/product"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// 装配时使用固定的资源边界。业务超时仍由 config.go 中的环境变量决定。
const (
	startupTimeout   = 30 * time.Second // 首次连接与迁移共用预算，依赖不可达时启动应失败。
	redisIOTimeout   = time.Second      // 短 Redis 操作应先于三秒 HTTP 预算结束。
	redisPoolSize    = 20               // 与默认数据库池同量级，避免本机压测无限扩连接。
	headerTimeout    = 3 * time.Second  // 请求头很小，限制慢速发送者占住连接的时间。
	idleTimeout      = 30 * time.Second // 短期复用 HTTP 连接，长期空闲后回收。
	processingMargin = time.Second      // 给时钟偏差和最后的状态写回留出余量。
	cleanupSteps     = 3                // 失败路径最多补偿、删占位、标记待核对；消息预算另算。
	shutdownMargin   = 5 * time.Second  // 在最慢业务路径之外给连接排空留余量。
)

// Run 的 result 是具名返回值，defer 可以把关闭连接的错误一并交给入口。
func Run(parent context.Context) (result error) {
	settings, err := loadConfig(config.New())
	if err != nil {
		return err
	}
	observability, err := telemetry.New(parent, "order-service")
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
	// Run 只持有整组依赖的关闭责任。具体连接顺序放在 dependencies.go，失败时也会清理。
	resources, err := openDependencies(startup, settings, observability, logger)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, resources.close()) }()
	httpServer, admin, processingBudget, err := servers(settings, resources, logger, observability)
	if err != nil {
		return err
	}
	cancel()
	logger.Info("订单服务配置已加载", "version", telemetry.Version, "http_addr", settings.HTTPAddress, "admin_addr", settings.AdminAddress,
		"rate_limit_rps", settings.RateRPS, "rate_limit_burst", settings.RateBurst, "request_timeout", settings.HTTPBudget.String(),
		"inventory_grpc_idle_timeout", settings.InventoryIdleTimeout.String(),
		"publish_timeout", settings.PublishBudget.String(),
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

// routes 最外层建立 span 并计时，连被限流拒绝的请求也能被观测到。
// 请求标识和恢复位于业务之前，确保普通错误响应也能带上关联信息。
func routes(settings Config, products *product.Service, orders *order.Service, logger *slog.Logger, observability *telemetry.Runtime) (*gin.Engine, error) {
	limit, err := resilience.RateLimit(settings.RateRPS, settings.RateBurst)
	if err != nil {
		return nil, err
	}
	router := gin.New()
	// 外层监控负责看见所有结果，内层保护负责把 panic、限流和超时变成明确响应。
	router.Use(
		otelgin.Middleware(observability.Service,
			otelgin.WithTracerProvider(observability.Provider), otelgin.WithPropagators(observability.Propagator)),
		observability.Metrics.HTTPMiddleware(),
		httpx.RequestIDs(logger), httpx.Recovery(logger), limit,
		httpx.BodyLimit(httpx.DefaultBodyLimit), resilience.RequestBudget(settings.HTTPBudget),
	)
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
