// 本文件：订单服务在启动前校验整个请求、库存调用和订单收尾的预算，并据此确定幂等占位的处理期限。

package orderservice

import (
	"errors"
	"fmt"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/resilience"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/app"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/inventoryclient"
)

// 空闲时间只控制连接资源回收，不是 RPC 的执行预算。默认值沿用 grpc-go，最大一天用于拦住明显误填。
const (
	defaultInventoryIdleTimeout = 30 * time.Minute
	maximumInventoryIdleTimeout = 24 * time.Hour
	defaultPublishTimeout       = 5 * time.Second // Kafka 单次写预算为三秒；确认等待还要容纳凑批、排队和一次短暂抖动。
)

// Config 中的 Database 含密码，禁止把整个结构体写进日志。
type Config struct {
	Database             mysqlx.Config
	HTTPAddress          string
	AdminAddress         string
	RedisPassword        string
	RedisAddress         string // 商品缓存和幂等记录共用 DB 0。
	KafkaBrokers         []string
	InventoryTarget      string
	InventoryIdleTimeout time.Duration // 设为零会一直保留空闲连接。
	Inventory            inventoryclient.Config
	HTTPBudget           time.Duration
	CleanupBudget        time.Duration
	PublishBudget        time.Duration
	IdempotencyTTL       time.Duration
	Singleflight         bool
	CacheJitter          time.Duration
	RateRPS              int
	RateBurst            int
}

// configValues 记下每个变量的校验错误，让启动日志一次列出所有需要修改的配置。
type configValues struct {
	reader   config.Reader
	problems []error
}

func (values *configValues) stringValue(name, fallback string) string {
	value, err := values.reader.String(name, fallback)
	values.problems = append(values.problems, err)
	return value
}

func (values *configValues) intValue(name string, fallback, minimum, maximum int) int {
	value, err := values.reader.Int(name, fallback, minimum, maximum)
	values.problems = append(values.problems, err)
	return value
}

func (values *configValues) durationValue(name string, fallback, minimum, maximum time.Duration) time.Duration {
	value, err := values.reader.Duration(name, fallback, minimum, maximum)
	values.problems = append(values.problems, err)
	return value
}

func (values *configValues) boolValue(name string, fallback bool) bool {
	value, err := values.reader.Bool(name, fallback)
	values.problems = append(values.problems, err)
	return value
}

func loadConfig(reader config.Reader) (Config, error) {
	values := configValues{reader: reader}
	database, err := app.DatabaseConfig(reader)
	values.problems = append(values.problems, err)
	value := Config{
		Database:             database,
		RedisPassword:        values.stringValue("REDIS_PASSWORD", ""),
		HTTPAddress:          values.stringValue("HTTP_ADDR", ":8081"),
		AdminAddress:         values.stringValue("ADMIN_ADDR", ":6061"),
		RedisAddress:         values.stringValue("REDIS_ADDR", "127.0.0.1:6379"),
		InventoryTarget:      values.stringValue("INVENTORY_GRPC_TARGET", "127.0.0.1:9001"),
		InventoryIdleTimeout: values.durationValue("INVENTORY_GRPC_IDLE_TIMEOUT", defaultInventoryIdleTimeout, 0, maximumInventoryIdleTimeout),
		HTTPBudget:           values.durationValue("HTTP_REQUEST_TIMEOUT", 3*time.Second, time.Millisecond, 30*time.Second),
		CleanupBudget:        values.durationValue("ORDER_CLEANUP_TIMEOUT", 2*time.Second, time.Millisecond, 5*time.Second),
		PublishBudget:        values.durationValue("ORDER_PUBLISH_TIMEOUT", defaultPublishTimeout, time.Millisecond, 10*time.Second),
		IdempotencyTTL:       values.durationValue("IDEMPOTENCY_TTL", 24*time.Hour, time.Minute, 7*24*time.Hour),
		Singleflight:         values.boolValue("CACHE_SINGLEFLIGHT", true),
		CacheJitter:          values.durationValue("CACHE_TTL_JITTER", time.Minute, 0, time.Minute),
		RateRPS:              values.intValue("RATE_LIMIT_RPS", 500, 1, 100000),
		RateBurst:            values.intValue("RATE_LIMIT_BURST", 100, 1, 100000),
		Inventory:            values.inventoryConfig(),
	}
	value.KafkaBrokers, err = app.KafkaBrokers(values.stringValue("KAFKA_BROKERS", "127.0.0.1:9092"))
	values.problems = append(values.problems, err)
	// gRPC 必须比上游更早结束，才能留出响应时间。补偿也必须容得下一次库存调用。
	if value.Inventory.Timeout >= value.HTTPBudget || value.Inventory.Timeout >= value.CleanupBudget {
		values.problems = append(values.problems, fmt.Errorf("库存调用超时必须小于 HTTP 请求预算和订单收尾预算"))
	}
	if value.Inventory.AttemptTimeout > value.Inventory.Timeout {
		values.problems = append(values.problems, fmt.Errorf("单次 GetStock 尝试时限不能长于库存调用总时限"))
	}
	if value.IdempotencyTTL <= processingBudget(value) || value.IdempotencyTTL%time.Second != 0 {
		values.problems = append(values.problems, fmt.Errorf("幂等保存时间须为整秒，并长于完整处理预算"))
	}
	return value, errors.Join(values.problems...)
}

func (values *configValues) inventoryConfig() inventoryclient.Config {
	return inventoryclient.Config{
		Timeout:        values.durationValue("INVENTORY_CALL_TIMEOUT", 800*time.Millisecond, time.Millisecond, 10*time.Second),
		AttemptTimeout: values.durationValue("INVENTORY_GETSTOCK_ATTEMPT_TIMEOUT", 0, 0, 10*time.Second),
		Retry: resilience.RetryPolicy{
			MaxRetries: values.intValue("INVENTORY_GETSTOCK_MAX_RETRIES", 2, 0, 10),
			Backoff:    values.durationValue("INVENTORY_RETRY_BACKOFF", 50*time.Millisecond, 0, time.Second),
			MinBudget:  values.durationValue("INVENTORY_RETRY_MIN_BUDGET", 50*time.Millisecond, time.Millisecond, time.Second),
		},
		Breaker: resilience.BreakerConfig{
			Enabled:        values.boolValue("INVENTORY_BREAKER_ENABLED", true),
			MinRequests:    uint32(values.intValue("INVENTORY_BREAKER_MIN_REQUESTS", 10, 1, 100000)),
			FailurePercent: uint32(values.intValue("INVENTORY_BREAKER_FAILURE_PERCENT", 50, 1, 100)),
			Interval:       values.durationValue("INVENTORY_BREAKER_INTERVAL", 10*time.Second, time.Millisecond, time.Hour),
			OpenTimeout:    values.durationValue("INVENTORY_BREAKER_OPEN_TIMEOUT", 5*time.Second, time.Millisecond, time.Minute),
			HalfOpenMax:    uint32(values.intValue("INVENTORY_BREAKER_HALF_OPEN_MAX", 1, 1, 100)),
		},
	}
}

// processingBudget 给幂等占位和优雅停机使用同一个上界，避免发送还在执行时就把请求判为待核对。
func processingBudget(settings Config) time.Duration {
	// 失败路径最多三次补偿/状态收尾；成功路径还可能等待消息确认。
	// 两条路径不会全部叠加，仍取一个便于理解的保守上界，给最后一次状态更新留余量。
	return settings.HTTPBudget + settings.PublishBudget + cleanupSteps*settings.CleanupBudget + processingMargin
}
