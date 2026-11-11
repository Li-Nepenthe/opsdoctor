// 本文件：订单进程用已经打开的连接组装商品与下单服务，再创建业务端口和管理端口。

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

type shopServices struct {
	products *product.Service
	orders   *order.Service
}

type httpServers struct {
	business         *http.Server
	admin            *http.Server
	processingBudget time.Duration
}

// services 在构造函数之间传递接口，让商品、订单包不用自己打开连接或注册全局指标。
func services(settings Config, resources *dependencies, runtime *telemetry.Runtime, logger *slog.Logger, budget time.Duration) (shopServices, error) {
	cache, err := product.NewRedisCache(resources.redis, settings.CacheJitter)
	if err != nil {
		return shopServices{}, err
	}
	products := product.NewService(
		&observedProducts{Repository: product.NewMySQLRepository(resources.database), metrics: runtime.Metrics},
		observedCache{Cache: cache, metrics: runtime.Metrics}, resources.stock, settings.Singleflight, logger,
	)
	// 只有整条业务路径和收尾预算都耗尽，旧 processing 占位才能转为待核对。
	idempotency, err := order.NewRedisIdempotency(resources.redis, settings.IdempotencyTTL, budget)
	if err != nil {
		return shopServices{}, err
	}
	orders := order.NewService(order.Dependencies{
		Repository:     order.NewMySQLRepository(resources.database),
		Prices:         products,
		Stock:          resources.stock,
		Publisher:      resources.publisher,
		Idempotency:    observedIdempotency{IdempotencyStore: idempotency, metrics: runtime.Metrics},
		Reviews:        newReviewRecorder(logger, runtime.Metrics),
		CleanupTimeout: settings.CleanupBudget,
		PublishTimeout: settings.PublishBudget,
		Logger:         logger,
	})
	return shopServices{products: products, orders: orders}, nil
}

// servers 只完成组装，serve 会在两个端口都绑定成功后才开放流量。
func servers(settings Config, resources *dependencies, logger *slog.Logger, runtime *telemetry.Runtime) (httpServers, error) {
	// 消息确认有自己的预算，幂等占位和 HTTP 排空必须一并容纳它。
	budget := processingBudget(settings)
	shop, err := services(settings, resources, runtime, logger, budget)
	if err != nil {
		return httpServers{}, err
	}
	router, err := routes(settings, shop.products, shop.orders, logger, runtime)
	if err != nil {
		return httpServers{}, err
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
	return httpServers{business: business, admin: admin, processingBudget: budget}, nil
}
