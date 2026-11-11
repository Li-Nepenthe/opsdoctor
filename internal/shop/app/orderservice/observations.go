// 本文件：在业务接口外记录缓存读取、重复回源和幂等占位的结果，让业务包独立于监控实现。

package orderservice

import (
	"context"
	"errors"
	"sync"

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
	product.Repository
	metrics *telemetry.Metrics // 只记次数，不把商品编号变成标签。
	mu      sync.Mutex         // 只保护在途计数，不能拿着这把锁执行数据库查询。
	active  map[int64]int      // 只保存还没查完的商品，最后一个查询结束就删除。
}

// Get 在调用仓储前记录一次回源尝试，包括后面的借连接和 SQL 执行。失败的尝试也保留。
// 同一商品已有仓储调用在途，说明发生了重叠；它不一定已同时进入数据库，只作调查证据。
func (repository *observedProducts) Get(ctx context.Context, id int64) (product.Product, error) {
	repository.metrics.CacheLoads.WithLabelValues("product").Inc()
	// 商品编号只留在进程内的临时表里，不能作为 Prometheus 标签，否则商品越多，指标就越多。
	repository.mu.Lock()
	if repository.active == nil {
		repository.active = make(map[int64]int)
	}
	concurrent := repository.active[id] > 0
	repository.active[id]++
	repository.mu.Unlock()
	concurrentCounter := repository.metrics.CacheConcurrentLoads.WithLabelValues("product")
	if concurrent {
		concurrentCounter.Inc()
	}
	defer func() {
		// 成功、查不到和数据库错误都要释放计数，不把结束的查询永久留成“仍在途”。
		repository.mu.Lock()
		defer repository.mu.Unlock()
		repository.active[id]--
		if repository.active[id] == 0 {
			delete(repository.active, id)
		}
	}()
	return repository.Repository.Get(ctx, id)
}

// observedIdempotency 不改变占位逻辑，其余完成、放弃和待核对动作仍由原实现处理。
type observedIdempotency struct {
	order.IdempotencyStore
	metrics *telemetry.Metrics // needs_review 由 reviewRecorder 在状态进入时统计。
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
