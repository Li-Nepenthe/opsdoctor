// 本文件：验证商品查询的缓存顺序、并发回源和库存降级。
//
// 内存替身只记录访问次数，测试因此可以证明有没有重复查询，而不依赖 Redis 或 MySQL 的速度。
//
// 主要内容：
//   - memoryCache、fakeRepository、fakeStock：提供可观察的测试依赖。
//   - TestCacheAside：检查命中、空值缓存和数据库故障。
//   - TestSingleflight：比较开启与关闭防护时的回源次数。
//   - TestDetailDegradation：库存失败仍返回商品，缺失的库存数值不写入 JSON。
//   - waitUntil：等待明确的并发条件，超时后让测试失败。
//
// 阅读建议：先看 CacheAside 的计数断言，再看并发测试如何挡住第一次查询。

package product

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/shop/inventory"
)

// cachedValue 同时保存正常值和不存在标记，模拟 Redis 中的 JSON 与 null。
type cachedValue struct {
	value   Product // 正常商品的数据。
	missing bool    // true 表示数据库已确认商品不存在。
}

// memoryCache 用锁保护 map，因为 singleflight 测试会同时读写它。
type memoryCache struct {
	mu     sync.Mutex            // 保护 values 和 writes。
	values map[int64]cachedValue // 按商品编号保存测试数据。
	writes int                   // 只在持锁期间修改。
	reads  atomic.Int32          // 并发测试用它等待所有请求进入缓存路径。
	err    error                 // 非空时模拟 Redis 无法访问，构造后不修改。
}

// Get 和真实缓存一样区分“没有记录”和“记录说商品不存在”。
func (cache *memoryCache) Get(_ context.Context, id int64) (Product, bool, error) {
	cache.reads.Add(1)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.err != nil {
		return Product{}, false, cache.err
	}
	value, ok := cache.values[id]
	if ok && value.missing {
		return Product{}, true, ErrNotFound
	}
	return value.value, ok, nil
}

// Set 记录是否回填。缓存故障时不假装写入成功。
func (cache *memoryCache) Set(_ context.Context, id int64, value Product, missing bool) error {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.err != nil {
		return cache.err
	}
	cache.writes++
	cache.values[id] = cachedValue{value: value, missing: missing}
	return nil
}

// fakeRepository 可以挡住查询，制造多个请求同时等待同一个商品的情况。
type fakeRepository struct {
	calls atomic.Int32    // 统计真正进入数据库替身的次数。
	value Product         // 要返回的商品。
	err   error           // 模拟不存在或数据库故障。
	gate  <-chan struct{} // 非 nil 时等测试放行，避免依赖固定睡眠时长。
}

// Get 等待时也尊重取消，防止测试结束后仍有查询留在后台。
func (repository *fakeRepository) Get(ctx context.Context, _ int64) (Product, error) {
	repository.calls.Add(1)
	if repository.gate != nil {
		select {
		case <-ctx.Done():
			return Product{}, ctx.Err()
		case <-repository.gate:
		}
	}
	return repository.value, repository.err
}

// fakeStock 只为商品展示提供库存结果，和数据库替身互不依赖。
type fakeStock struct{ err error } // err 非空时模拟下游不可用。

// GetStock 正常时返回零，专门验证“零库存”不会因 omitempty 消失。
func (stock fakeStock) GetStock(context.Context, int64) (inventory.Stock, error) {
	return inventory.Stock{SKUID: 1, Available: 0}, stock.err
}

// quietLogger 避免预期故障日志淹没测试断言。
func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// TestCacheAside 连续查两次，第二次是否访问数据库由第一次得到的结果决定。
func TestCacheAside(t *testing.T) {
	for _, testcase := range []struct {
		name          string // 说明数据库与缓存分别处于什么状态。
		repositoryErr error  // 数据库返回的结果分类。
		cacheErr      error  // 模拟 Redis 故障。
		calls         int32  // 两次业务查询应产生几次数据库查询。
		writes        int    // 预期成功写入缓存的次数。
	}{
		{"正常回填", nil, nil, 1, 1},                       // 第二次直接使用缓存。
		{"不存在回填空值", ErrNotFound, nil, 1, 1},            // 第二次也不必再问数据库。
		{"数据库故障不缓存", errors.New("连接失败"), nil, 2, 0},    // 错误不能缓存成商品不存在。
		{"缓存不可用回源", nil, errors.New("Redis 断开"), 2, 0}, // 商品查询允许退回数据库。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			repository := &fakeRepository{value: Product{ID: 1, Name: "商品", PriceCents: 100}, err: testcase.repositoryErr}
			cache := &memoryCache{values: make(map[int64]cachedValue), err: testcase.cacheErr}
			service := NewService(repository, cache, fakeStock{}, true, quietLogger())
			for range 2 {
				_, err := service.Get(t.Context(), 1)
				if !errors.Is(err, testcase.repositoryErr) {
					t.Fatalf("错误被缓存改变：%v", err)
				}
			}
			if repository.calls.Load() != testcase.calls || cache.writes != testcase.writes {
				t.Fatalf("回源=%d 写缓存=%d", repository.calls.Load(), cache.writes)
			}
		})
	}
}

// TestSingleflight 让五十个请求同时遇到冷缓存，防护开启时数据库只处理一份查询。
func TestSingleflight(t *testing.T) {
	for _, merge := range []bool{true, false} {
		name := "关闭防护"
		if merge {
			name = "开启防护"
		}
		t.Run(name, func(t *testing.T) {
			const requests = 50 // 按阶段验收要求制造五十个重叠请求，不依赖机器线程数。
			gate := make(chan struct{})
			repository := &fakeRepository{value: Product{ID: 1, PriceCents: 100}, gate: gate}
			cache := &memoryCache{values: make(map[int64]cachedValue)}
			service := NewService(repository, cache, fakeStock{}, merge, quietLogger())
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			results := make(chan error, requests)
			for range requests {
				go func() { _, err := service.Get(ctx, 1); results <- err }()
			}
			wanted := int32(requests)
			if merge {
				wanted = 1
			}
			// 等所有请求先看过冷缓存，再放行数据库。关闭防护时还要等五十次回源都进入。
			waitUntil(t, func() bool { return cache.reads.Load() >= requests+1 && repository.calls.Load() == wanted })
			close(gate)
			for range requests {
				if err := <-results; err != nil {
					t.Fatal(err)
				}
			}
			if repository.calls.Load() != wanted {
				t.Fatalf("回源次数=%d，期望 %d", repository.calls.Load(), wanted)
			}
		})
	}
}

// TestDetailDegradation 从 JSON 结果检查降级，避免只看 Go 结构体而漏掉契约问题。
func TestDetailDegradation(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		repository := &fakeRepository{value: Product{ID: 1, PriceCents: 100}}
		cache := &memoryCache{values: make(map[int64]cachedValue)}
		stock := fakeStock{}
		if unavailable {
			stock.err = inventory.ErrUnavailable
		}
		service := NewService(repository, cache, stock, true, quietLogger())
		detail, err := service.Detail(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(detail)
		if err != nil {
			t.Fatal(err)
		}
		// 正常的零库存必须输出 available:0。降级时整个字段省略。
		if strings.Contains(string(data), `"available":0`) == unavailable {
			t.Fatalf("库存表示不对：%s", data)
		}
		if unavailable && detail.Stock.Status != "unavailable" {
			t.Fatal("缺少降级标记")
		}
	}
}

// waitUntil 等待可观察的条件，限定五秒，避免并发错误把测试永久挂住。
func waitUntil(t *testing.T, ready func() bool) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !ready() {
		select {
		case <-timer.C:
			t.Fatal("没有达到并发测试的等待条件")
		case <-tick.C:
		}
	}
}
