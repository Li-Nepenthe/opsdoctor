// 本文件：在真实 Redis 中检查商品缓存的内容和过期规则。
//
// 测试固定用 14 号数据库，使用独立商品编号，只删除自己的 key，不清空其他记录。
//
// 主要内容：
//   - TestIntegrationRedisCache：检查未命中、正常商品、空值和 TTL。
//
// 阅读建议：把这里观察到的 Redis 行为与 cache_test.go 的纯函数测试对照。

package product

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestIntegrationRedisCache 先写正常商品再覆盖为空值，两种状态都必须能被下一次读取识别。
func TestIntegrationRedisCache(t *testing.T) {
	address := os.Getenv("TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("设置 TEST_REDIS_ADDR 后运行商品缓存测试，固定使用数据库 14")
	}
	client := redis.NewClient(&redis.Options{Addr: address, DB: 14, MaxRetries: -1, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second, ContextTimeoutEnabled: true})
	id := time.Now().UnixNano()
	key := "product:" + strconv.FormatInt(id, 10)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := client.Del(ctx, key).Err(); err != nil {
			t.Error(err)
		}
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	cache, err := NewRedisCache(client, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, hit, err := cache.Get(t.Context(), id); err != nil || hit {
		t.Fatalf("新键应未命中：%t %v", hit, err)
	}
	want := Product{ID: id, Name: "缓存测试商品", PriceCents: 100}
	if err := cache.Set(t.Context(), id, want, false); err != nil {
		t.Fatal(err)
	}
	value, hit, err := cache.Get(t.Context(), id)
	if err != nil || !hit || value != want {
		t.Fatalf("商品缓存结果：%+v %t %v", value, hit, err)
	}
	ttl := client.PTTL(t.Context(), key).Val()
	// 留一秒给真实网络往返。TTL 上界仍不得超过计划书的一分钟抖动。
	if ttl < 10*time.Minute-time.Second || ttl > 11*time.Minute {
		t.Fatalf("正常商品 TTL=%v", ttl)
	}
	if err := cache.Set(t.Context(), id, Product{}, true); err != nil {
		t.Fatal(err)
	}
	_, hit, err = cache.Get(t.Context(), id)
	if !hit || !errors.Is(err, ErrNotFound) {
		t.Fatalf("空值缓存结果：%t %v", hit, err)
	}
	ttl = client.PTTL(t.Context(), key).Val()
	if ttl < 29*time.Second || ttl > 30*time.Second {
		t.Fatalf("空值 TTL=%v", ttl)
	}
}
