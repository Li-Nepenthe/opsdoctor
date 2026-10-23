// 本文件：检查商品缓存的内容和有效时间。
//
// 这里直接测试编码解析和时间计算，不需要 Redis。真实 Redis 的读写由集成测试覆盖。
//
// 主要内容：
//   - TestDecodeProduct：区分正常商品、空值、损坏内容和错误编号。
//   - TestExpiration：确认空值不抖动、关闭抖动后固定十分钟。
//
// 阅读建议：先看每种缓存内容是命中还是未命中，再看它是否允许回源。

package product

import (
	"errors"
	"testing"
	"time"
)

// TestDecodeProduct 防止损坏内容被当成价格为零的有效商品。
func TestDecodeProduct(t *testing.T) {
	for _, testcase := range []struct {
		name    string // 描述缓存内容属于哪一类。
		data    string // 模拟 Redis 返回的原始 JSON。
		hit     bool   // true 表示可以使用缓存结果。
		missing bool   // 只有 null 才能明确表示不存在。
	}{
		{"正常", `{"id":1,"name":"商品","price_cents":100}`, true, false}, // 合法商品可以直接使用。
		{"空值", `null`, true, true},                                    // 不存在也缓存，避免无效编号反复回源。
		{"损坏", `{`, false, false},                                     // JSON 无法解析时由数据库修复。
		{"错编号", `{"id":2,"price_cents":100}`, false, false},           // 防止串到另一件商品的价格。
		{"负价格", `{"id":1,"price_cents":-1}`, false, false},            // 异常价格不能继续下单。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			_, hit, err := decodeProduct([]byte(testcase.data), 1)
			if hit != testcase.hit || errors.Is(err, ErrNotFound) != testcase.missing {
				t.Fatalf("hit=%t err=%v", hit, err)
			}
		})
	}
}

// TestExpiration 多取几次随机值检查范围，不要求随机数恰好出现某个特定值。
func TestExpiration(t *testing.T) {
	cache := &RedisCache{jitter: time.Minute}
	for range 100 {
		value := cache.expiration(false)
		if value < 10*time.Minute || value > 11*time.Minute {
			t.Fatalf("TTL 越界：%v", value)
		}
		if cache.expiration(true) != 30*time.Second {
			t.Fatal("空值缓存不应增加随机时间")
		}
	}
	cache.jitter = 0
	if cache.expiration(false) != 10*time.Minute {
		t.Fatal("关闭抖动后 TTL 不固定")
	}
}
