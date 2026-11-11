// 本文件：分别缓存正常商品和查无此商品的结果。正常商品的过期时间加随机抖动，减少集中回源。
// 从 Get 可以区分 Redis 中没有键与已知查无商品，写入期限由 expiration 决定。

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

// RedisCache 复用订单服务的客户端，正常商品每次写入时单独取抖动值。
type RedisCache struct {
	client *redis.Client
	jitter time.Duration // 为 0 时关闭抖动，最大允许一分钟。
}

// NewRedisCache 在启动时校验抖动范围。
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
