// 本文件：在订单服务的接口外面加计数，不把 Prometheus 类型传进业务流程。
//
// 这些包装器仍实现原接口。它们先调用真实依赖，再根据结果记一次指标。
// 这样 product 和 order 仍然只描述业务顺序，具体监控方式由 app 决定。
//
// 主要内容：
//   - observedCache.Get：区分正常命中、空值命中、未命中和读取失败。
//   - observedProducts.Get：统计真正查询数据库的次数。
//   - observedIdempotency.Reserve：统计新请求、回放和处理中的重复请求。
//
// 阅读建议：嵌入的接口会转发没改写的方法，例如 Cache.Set；这里只包需要计数的动作。

package orderservice

import (
	"context"
	"errors"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/order"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/product"
)

// observedCache 统计每次缓存读取，第二次检查缓存也会计数，因此它不等于 HTTP 请求数。
type observedCache struct {
	product.Cache                    // Set 保持原来的 TTL 和错误行为。
	metrics       *telemetry.Metrics // 标签只有固定缓存名和四种结果。
}

// Get 把读取失败单独记为 error，不让 Redis 故障看起来只是普通未命中。
func (cache observedCache) Get(ctx context.Context, id int64) (product.Product, bool, error) {
	value, hit, err := cache.Cache.Get(ctx, id)
	result := "miss"
	switch {
	case hit && errors.Is(err, product.ErrNotFound):
		result = "null_hit" // 空值缓存已经回答了“商品不存在”，无需再查数据库。
	case err != nil:
		result = "error"
	case hit:
		result = "hit"
	}
	cache.metrics.CacheRequests.WithLabelValues("product", result).Inc()
	return value, hit, err
}

// observedProducts 放在 singleflight 后面的仓储接口上，合并后的查询只计一次回源。
type observedProducts struct {
	product.Repository                    // 实际查询仍使用原 MySQL 仓储。
	metrics            *telemetry.Metrics // 只记次数，不把商品编号变成标签。
}

// Get 在真正查库前计数，查询失败也算给数据库带来过一次请求。
func (repository observedProducts) Get(ctx context.Context, id int64) (product.Product, error) {
	repository.metrics.CacheLoads.WithLabelValues("product").Inc()
	return repository.Repository.Get(ctx, id)
}

// observedIdempotency 不改变占位逻辑，其余完成、放弃和待核对动作仍由原实现处理。
type observedIdempotency struct {
	order.IdempotencyStore                    // 嵌入接口，不复制 Redis 的原子状态转换。
	metrics                *telemetry.Metrics // needs_review 由 reviewRecorder 在状态进入时统计。
}

// Reserve 只统计成功读到的确定状态。读取失败不能冒充新请求或正在处理。
func (store observedIdempotency) Reserve(ctx context.Context, key string, request order.CreateRequest) (order.Reservation, bool, error) {
	reservation, owned, err := store.IdempotencyStore.Reserve(ctx, key, request)
	if err == nil {
		result := ""
		switch {
		case owned:
			result = "new"
		case reservation.State == "completed" && reservation.Result != nil:
			result = "replay"
		case reservation.State == "processing":
			result = "in_progress"
		}
		// 待核对重复读取不再加一次，只有第一次进入该状态才算一次告警。
		if result != "" {
			store.metrics.Idempotency.WithLabelValues(result).Inc()
		}
	}
	return reservation, owned, err
}
