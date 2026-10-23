// 本文件：读取订单进程的配置，并检查各层时间预算。
//
// Run 在连接外部服务前调用 loadConfig。参数错误应在启动时说明，不留到第一笔订单才发现。
//
// 主要内容：
//   - Config：列出这个进程真正使用的参数。
//   - loadConfig：读取环境变量，检查下游超时与上游预算的关系。
//   - kafkaBrokers：拆分地址并拒绝空项。
//
// 阅读建议：先看 HTTPBudget、CleanupBudget、Inventory.Timeout 的关系，再看它们如何决定旧占位的处理预算。

package orderservice

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/resilience"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/app"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/inventoryclient"
)

// Config 中的 Database 含密码，禁止把整个结构体写进日志。
type Config struct {
	Database        mysqlx.Config          // MYSQL_DSN 与连接池设置。
	HTTPAddress     string                 // HTTP_ADDR，默认 :8081。
	AdminAddress    string                 // ADMIN_ADDR，默认 :6061。
	RedisAddress    string                 // REDIS_ADDR，运行时缓存与幂等记录共用 DB 0。
	KafkaBrokers    []string               // KAFKA_BROKERS，逗号分隔后逐项检查。
	InventoryTarget string                 // INVENTORY_GRPC_TARGET，复用一条 gRPC 连接。
	Inventory       inventoryclient.Config // 库存调用的超时、重试和熔断参数。
	HTTPBudget      time.Duration          // HTTP_REQUEST_TIMEOUT，整次请求的上游预算。
	CleanupBudget   time.Duration          // ORDER_CLEANUP_TIMEOUT，每次补偿或状态收尾的预算。
	IdempotencyTTL  time.Duration          // IDEMPOTENCY_TTL，默认 24 小时。
	Singleflight    bool                   // CACHE_SINGLEFLIGHT，是否合并同商品回源。
	CacheJitter     time.Duration          // CACHE_TTL_JITTER，0 表示关闭随机过期增量。
	RateRPS         int                    // RATE_LIMIT_RPS，令牌每秒补充数。
	RateBurst       int                    // RATE_LIMIT_BURST，桶最多保存的令牌数。
}

// loadConfig 用局部读取函数收集错误，用户一次就能看到多个配置问题。
func loadConfig(reader config.Reader) (Config, error) {
	var problems []error
	readString := func(name, fallback string) string {
		value, err := reader.String(name, fallback)
		problems = append(problems, err)
		return value
	}
	readInt := func(name string, fallback, minimum, maximum int) int {
		value, err := reader.Int(name, fallback, minimum, maximum)
		problems = append(problems, err)
		return value
	}
	readDuration := func(name string, fallback, minimum, maximum time.Duration) time.Duration {
		value, err := reader.Duration(name, fallback, minimum, maximum)
		problems = append(problems, err)
		return value
	}
	readBool := func(name string, fallback bool) bool {
		value, err := reader.Bool(name, fallback)
		problems = append(problems, err)
		return value
	}
	database, err := app.DatabaseConfig(reader)
	problems = append(problems, err)
	value := Config{Database: database,
		HTTPAddress: readString("HTTP_ADDR", ":8081"), AdminAddress: readString("ADMIN_ADDR", ":6061"),
		RedisAddress: readString("REDIS_ADDR", "127.0.0.1:6379"), InventoryTarget: readString("INVENTORY_GRPC_TARGET", "127.0.0.1:9001"),
		HTTPBudget:     readDuration("HTTP_REQUEST_TIMEOUT", 3*time.Second, time.Millisecond, 30*time.Second),
		CleanupBudget:  readDuration("ORDER_CLEANUP_TIMEOUT", 2*time.Second, time.Millisecond, 5*time.Second),
		IdempotencyTTL: readDuration("IDEMPOTENCY_TTL", 24*time.Hour, time.Minute, 7*24*time.Hour),
		Singleflight:   readBool("CACHE_SINGLEFLIGHT", true), CacheJitter: readDuration("CACHE_TTL_JITTER", time.Minute, 0, time.Minute),
		RateRPS: readInt("RATE_LIMIT_RPS", 500, 1, 100000), RateBurst: readInt("RATE_LIMIT_BURST", 100, 1, 100000),
		Inventory: inventoryclient.Config{Timeout: readDuration("INVENTORY_CALL_TIMEOUT", 800*time.Millisecond, time.Millisecond, 10*time.Second),
			Retry:   resilience.RetryPolicy{MaxRetries: readInt("INVENTORY_GETSTOCK_MAX_RETRIES", 2, 0, 2), Backoff: readDuration("INVENTORY_RETRY_BACKOFF", 50*time.Millisecond, 0, time.Second), MinBudget: readDuration("INVENTORY_RETRY_MIN_BUDGET", 50*time.Millisecond, time.Millisecond, time.Second)},
			Breaker: resilience.BreakerConfig{Enabled: readBool("INVENTORY_BREAKER_ENABLED", true), MinRequests: uint32(readInt("INVENTORY_BREAKER_MIN_REQUESTS", 10, 1, 100000)), FailurePercent: uint32(readInt("INVENTORY_BREAKER_FAILURE_PERCENT", 50, 1, 100)), Interval: readDuration("INVENTORY_BREAKER_INTERVAL", 10*time.Second, time.Millisecond, time.Hour), OpenTimeout: readDuration("INVENTORY_BREAKER_OPEN_TIMEOUT", 5*time.Second, time.Millisecond, time.Minute), HalfOpenMax: uint32(readInt("INVENTORY_BREAKER_HALF_OPEN_MAX", 1, 1, 100))}},
	}
	value.KafkaBrokers, err = kafkaBrokers(readString("KAFKA_BROKERS", "127.0.0.1:9092"))
	problems = append(problems, err)
	// gRPC 必须比上游更早结束，才能留出响应时间。补偿也必须容得下一次库存调用。
	if value.Inventory.Timeout >= value.HTTPBudget || value.Inventory.Timeout >= value.CleanupBudget {
		problems = append(problems, fmt.Errorf("库存调用超时必须小于 HTTP 请求预算和订单收尾预算"))
	}
	if value.IdempotencyTTL <= value.HTTPBudget+cleanupSteps*value.CleanupBudget+processingMargin || value.IdempotencyTTL%time.Second != 0 {
		problems = append(problems, fmt.Errorf("幂等保存时间须为整秒，并长于完整处理预算"))
	}
	return value, errors.Join(problems...)
}

// kafkaBrokers 要求每项都包含主机和端口，空列表项通常是配置多写了一个逗号。
func kafkaBrokers(raw string) ([]string, error) {
	brokers := strings.Split(raw, ",")
	for index, address := range brokers {
		address = strings.TrimSpace(address)
		if host, port, err := net.SplitHostPort(address); err != nil || host == "" || port == "" {
			return nil, fmt.Errorf("KAFKA_BROKERS 必须是逗号分隔的主机:端口")
		}
		brokers[index] = address
	}
	return brokers, nil
}
