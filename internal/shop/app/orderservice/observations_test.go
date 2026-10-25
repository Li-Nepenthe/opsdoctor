// 本文件：检查同键并发回源只记录真实重叠的数据库查询。
//
// 用通道暂时挡住假的数据库查询，便能明确安排“先有一个在查，再来一个同商品查询”。
// 不依赖机器快慢，也不用连接真实 MySQL。
//
// 主要内容：
//   - productQuery：把测试函数适配成商品仓储。
//   - TestConcurrentProductLoads：区分同商品与不同商品，并检查查询结束后删除临时计数。
//
// 阅读建议：总回源三次，只有第二次同商品查询重叠，因此诊断计数应为一。

package orderservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/product"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// productQuery 让测试控制一次查询何时开始和结束，不另建数据库服务。
type productQuery func(context.Context, int64) (product.Product, error)

// Get 保留仓储接口的输入输出，监控包装器仍走生产中的调用顺序。
func (query productQuery) Get(ctx context.Context, id int64) (product.Product, error) {
	return query(ctx, id)
}

// TestConcurrentProductLoads 也让一个查询返回错误，证明失败不会遗留“仍在途”的商品。
func TestConcurrentProductLoads(t *testing.T) {
	metrics := telemetry.NewMetrics("order-service-test")
	entered, release := make(chan int64, 3), make(chan struct{})
	t.Cleanup(func() { close(release) })
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	repository := &observedProducts{metrics: metrics, Repository: productQuery(func(ctx context.Context, id int64) (product.Product, error) {
		entered <- id
		select {
		case <-ctx.Done():
			return product.Product{}, ctx.Err()
		case <-release:
		}
		if id == 2 {
			return product.Product{}, product.ErrNotFound
		}
		return product.Product{ID: id}, nil
	})}
	finished := make(chan error, 3)
	for _, id := range []int64{1, 1, 2} {
		// 确认上一项已经进入数据库，再发下一项，保证这三个查询确实重叠。
		go func() { _, err := repository.Get(ctx, id); finished <- err }()
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("查询没有进入假的数据库")
		}
	}
	if testutil.ToFloat64(metrics.CacheLoads.WithLabelValues("product")) != 3 || testutil.ToFloat64(metrics.CacheConcurrentLoads.WithLabelValues("product")) != 1 {
		t.Error("不同商品的并发或第一次查询被误算成重复回源")
	}
	for range 3 {
		release <- struct{}{}
	}
	for range 3 {
		if err := <-finished; err != nil && !errors.Is(err, product.ErrNotFound) {
			t.Error(err)
		}
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if len(repository.active) != 0 {
		t.Fatal("已经完成的商品仍留在临时表中")
	}
}
