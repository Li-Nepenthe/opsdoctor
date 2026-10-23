// 本文件：保存商品缓存，并把过期时间打散。
//
// service 决定何时读写缓存，本文件只负责 Redis 编码和过期规则。
// TTL 就是记录还能保存多久。加一点随机时间，可以避免一批商品同时失效。
//
// 主要内容：
//   - NewRedisCache：检查随机时间范围，复用 Redis 客户端。
//   - Get、decodeProduct：识别未命中、空值和正常商品。
//   - Set、expiration：写入 JSON，并选择正常商品或空值的过期时间。
//
// 阅读建议：先区分 Redis 没有 key 和 key 里保存 null，这两种情况处理不同。

package product

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// 缓存时长来自计划书。空值短一些，避免新增商品长时间不可见。
const (
	productTTL = 10 * time.Minute
	missingTTL = 30 * time.Second
)

// RedisCache 构造后只读 jitter，每次 Set 单独计算随机时间。
type RedisCache struct {
	client *redis.Client // app 负责创建、探活和关闭客户端。
	jitter time.Duration // 为 0 时关闭抖动，最大允许一分钟。
}

// NewRedisCache 把配置错误留在启动阶段，不让请求走到一半才失败。
func NewRedisCache(client *redis.Client, jitter time.Duration) (*RedisCache, error) {
	if client == nil || jitter < 0 || jitter > time.Minute {
		return nil, fmt.Errorf("商品缓存需要 Redis 客户端，抖动范围须为 0..60s")
	}
	return &RedisCache{client: client, jitter: jitter}, nil
}

// Get 遇到损坏的 JSON 也按未命中处理，让数据库有机会修复这条缓存。
func (cache *RedisCache) Get(ctx context.Context, id int64) (Product, bool, error) {
	data, err := cache.client.Get(ctx, "product:"+strconv.FormatInt(id, 10)).Bytes()
	if errors.Is(err, redis.Nil) {
		return Product{}, false, nil
	}
	if err != nil {
		return Product{}, false, err
	}
	return decodeProduct(data, id)
}

// decodeProduct 验证缓存编号，避免错误写入的值让一个商品返回另一个商品的价格。
func decodeProduct(data []byte, id int64) (Product, bool, error) {
	if string(data) == "null" {
		return Product{}, true, ErrNotFound
	}
	var value Product
	if err := json.Unmarshal(data, &value); err != nil {
		return Product{}, false, err
	}
	if value.ID != id || value.PriceCents < 0 {
		return Product{}, false, fmt.Errorf("缓存商品字段不合法")
	}
	return value, true, nil
}

// Set 用 JSON null 表示确实查不到，后续请求就不必反复查询同一个不存在的编号。
func (cache *RedisCache) Set(ctx context.Context, id int64, value Product, missing bool) error {
	data := []byte("null")
	if !missing {
		var err error
		data, err = json.Marshal(value)
		if err != nil {
			return fmt.Errorf("编码商品缓存：%w", err)
		}
	}
	return cache.client.Set(ctx, "product:"+strconv.FormatInt(id, 10), data, cache.expiration(missing)).Err()
}

// expiration 对空值始终使用三十秒。正常商品才需要打散十分钟后的回源时间。
func (cache *RedisCache) expiration(missing bool) time.Duration {
	if missing {
		return missingTTL
	}
	if cache.jitter == 0 {
		return productTTL
	}
	return productTTL + time.Duration(rand.Int64N(int64(cache.jitter)+1))
}
