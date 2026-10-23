// 本文件：用已经准备好的依赖装配业务，并创建两个 HTTP 入口。
//
// dependencies.go 只关心连接。这里把连接交给仓储，再把仓储和客户端交给业务服务。
// 监控包装也在这一层安装，业务规则不需要知道 Prometheus 的接口。
//
// 主要内容：
//   - services：装配商品缓存、订单幂等和待核对记录。
//   - servers：创建业务与管理服务器，统一设置请求和停机预算。
//
// 阅读建议：processingBudget 包含客户端断开后仍要完成的收尾，不能只用 HTTPBudget。

package orderservice

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/adminserver"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/order"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/product"
)

// services 在构造函数之间传递接口，让商品、订单包不用自己打开连接或注册全局指标。
func services(settings Config, resources *dependencies, runtime *telemetry.Runtime, logger *slog.Logger, budget time.Duration) (*product.Service, *order.Service, error) {
	cache, err := product.NewRedisCache(resources.redis, settings.CacheJitter)
	if err != nil {
		return nil, nil, err
	}
	products := product.NewService(
		observedProducts{Repository: product.NewMySQLRepository(resources.database), metrics: runtime.Metrics},
		observedCache{Cache: cache, metrics: runtime.Metrics}, resources.stock, settings.Singleflight, logger,
	)
	// 只有整条业务路径和收尾预算都耗尽，旧 processing 占位才能转为待核对。
	idempotency, err := order.NewRedisIdempotency(resources.redis, settings.IdempotencyTTL, budget)
	if err != nil {
		return nil, nil, err
	}
	orders := order.NewService(
		order.NewMySQLRepository(resources.database), products, resources.stock, resources.publisher,
		observedIdempotency{IdempotencyStore: idempotency, metrics: runtime.Metrics},
		newReviewRecorder(logger, runtime.Metrics), settings.CleanupBudget, logger,
	)
	return products, orders, nil
}

// servers 只创建对象，不开始监听。serve 会先确认两个端口都能绑定，再开放流量。
func servers(settings Config, resources *dependencies, logger *slog.Logger, runtime *telemetry.Runtime) (*http.Server, *http.Server, time.Duration, error) {
	// 成功路径也可能依次发消息、保存结果、标记待核对，最多使用三次独立收尾预算。
	budget := settings.HTTPBudget + cleanupSteps*settings.CleanupBudget + processingMargin
	products, orders, err := services(settings, resources, runtime, logger, budget)
	if err != nil {
		return nil, nil, 0, err
	}
	router, err := routes(settings, products, orders, logger, runtime)
	if err != nil {
		return nil, nil, 0, err
	}
	business := &http.Server{
		Addr: settings.HTTPAddress, Handler: router, ReadHeaderTimeout: headerTimeout,
		ReadTimeout: settings.HTTPBudget, WriteTimeout: budget + processingMargin, IdleTimeout: idleTimeout,
	}
	admin := adminserver.New(settings.AdminAddress, func(probe context.Context) error {
		if err := resources.database.PingContext(probe); err != nil {
			return err
		}
		return resources.redis.Ping(probe).Err()
	}, runtime.Metrics.Registry)
	return business, admin, budget, nil
}
