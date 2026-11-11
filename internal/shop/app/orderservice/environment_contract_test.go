// 本文件：用进程自己的配置加载函数固定变量名、默认值和独立校验范围。

package orderservice

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestOrderServiceEnvironmentContract 记录实际查找的变量，用合法的测试凭据走完配置读取，再对每个名字单独传入错误值。
func TestOrderServiceEnvironmentContract(t *testing.T) {
	supplied := map[string]string{"MYSQL_DSN": "test-only", "REDIS_PASSWORD": "test-only"}
	keys := map[string]bool{}
	read := func(override, raw string) config.Reader {
		return config.NewReader(func(name string) (string, bool) {
			keys[name] = true
			if name == override {
				return raw, true
			}
			v, ok := supplied[name]
			return v, ok
		})
	}
	settings, err := loadConfig(read("", ""))
	if err != nil {
		t.Fatal(err)
	}
	defaults := map[string]any{
		"MYSQL_DSN":                          settings.Database.DSN,
		"DB_MAX_OPEN_CONNS":                  settings.Database.MaxOpenConns,
		"DB_MAX_IDLE_CONNS":                  settings.Database.MaxIdleConns,
		"DB_CONN_MAX_LIFETIME":               settings.Database.ConnMaxLifetime,
		"HTTP_ADDR":                          settings.HTTPAddress,
		"ADMIN_ADDR":                         settings.AdminAddress,
		"REDIS_PASSWORD":                     settings.RedisPassword,
		"REDIS_ADDR":                         settings.RedisAddress,
		"KAFKA_BROKERS":                      settings.KafkaBrokers,
		"INVENTORY_GRPC_TARGET":              settings.InventoryTarget,
		"INVENTORY_GRPC_IDLE_TIMEOUT":        settings.InventoryIdleTimeout,
		"HTTP_REQUEST_TIMEOUT":               settings.HTTPBudget,
		"ORDER_CLEANUP_TIMEOUT":              settings.CleanupBudget,
		"ORDER_PUBLISH_TIMEOUT":              settings.PublishBudget,
		"IDEMPOTENCY_TTL":                    settings.IdempotencyTTL,
		"CACHE_SINGLEFLIGHT":                 settings.Singleflight,
		"CACHE_TTL_JITTER":                   settings.CacheJitter,
		"RATE_LIMIT_RPS":                     settings.RateRPS,
		"RATE_LIMIT_BURST":                   settings.RateBurst,
		"INVENTORY_CALL_TIMEOUT":             settings.Inventory.Timeout,
		"INVENTORY_GETSTOCK_ATTEMPT_TIMEOUT": settings.Inventory.AttemptTimeout,
		"INVENTORY_GETSTOCK_MAX_RETRIES":     settings.Inventory.Retry.MaxRetries,
		"INVENTORY_RETRY_BACKOFF":            settings.Inventory.Retry.Backoff,
		"INVENTORY_RETRY_MIN_BUDGET":         settings.Inventory.Retry.MinBudget,
		"INVENTORY_BREAKER_ENABLED":          settings.Inventory.Breaker.Enabled,
		"INVENTORY_BREAKER_MIN_REQUESTS":     settings.Inventory.Breaker.MinRequests,
		"INVENTORY_BREAKER_FAILURE_PERCENT":  settings.Inventory.Breaker.FailurePercent,
		"INVENTORY_BREAKER_INTERVAL":         settings.Inventory.Breaker.Interval,
		"INVENTORY_BREAKER_OPEN_TIMEOUT":     settings.Inventory.Breaker.OpenTimeout,
		"INVENTORY_BREAKER_HALF_OPEN_MAX":    settings.Inventory.Breaker.HalfOpenMax,
	}

	checkOrderServiceRanges(t, read)
	var report strings.Builder
	for _, name := range slices.Sorted(maps.Keys(keys)) {
		// 字符串的值从装配结果读取；范围来自同一个变量被错误输入拒绝时的诊断。
		fallback, ok := defaults[name]
		if !ok {
			t.Fatalf("新出现的环境变量 %s 没有默认值断言", name)
		}
		if value, required := supplied[name]; required {
			// 有了部署值才允许继续读取其余默认值；单独省略它要得到必填错误。
			delete(supplied, name)
			_, missing := loadConfig(read("", ""))
			supplied[name] = value
			if missing == nil || !strings.Contains(missing.Error(), "环境变量 "+name+" 不能为空") {
				t.Fatalf("%s 不再要求配置", name)
			}
			fallback = "(必填)"
		}
		_, problem := loadConfig(read(name, "!"))
		message := "—"
		if problem != nil {
			message = strings.ReplaceAll(problem.Error(), "\n", " | ")
		}
		fmt.Fprintf(&report, "%s\t%v\t%s\n", name, fallback, message)
	}
	want, err := os.ReadFile("testdata/environment.golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	if report.String() != string(want) {
		t.Fatalf("环境配置的名字、默认值或拒绝规则发生变化：\n%s", report.String())
	}
}

// checkOrderServiceRanges 验证边界值及紧邻的越界值。
func checkOrderServiceRanges(t *testing.T, read func(string, string) config.Reader) {
	t.Helper()
	// 查看该变量自己的范围错误。其他字段之间的预算约束在独立测试中验证。
	checkRange := func(name, raw string, inside bool) {
		t.Helper()
		_, err := loadConfig(read(name, raw))
		rejected := err != nil && strings.Contains(err.Error(), "环境变量 "+name+" 必须是")
		if rejected == inside {
			t.Fatalf("%s=%s，期望在范围内=%t：%v", name, raw, inside, err)
		}
	}
	for _, bound := range []struct {
		name     string
		min, max int
	}{
		{"DB_MAX_IDLE_CONNS", 0, 20},
		{"DB_MAX_OPEN_CONNS", 1, 100},
		{"INVENTORY_BREAKER_FAILURE_PERCENT", 1, 100},
		{"INVENTORY_BREAKER_HALF_OPEN_MAX", 1, 100},
		{"INVENTORY_BREAKER_MIN_REQUESTS", 1, 100000},
		{"INVENTORY_GETSTOCK_MAX_RETRIES", 0, 10},
		{"RATE_LIMIT_BURST", 1, 100000},
		{"RATE_LIMIT_RPS", 1, 100000},
	} {
		checkRange(bound.name, strconv.Itoa(bound.min-1), false)
		checkRange(bound.name, strconv.Itoa(bound.min), true)
		checkRange(bound.name, strconv.Itoa(bound.max), true)
		checkRange(bound.name, strconv.Itoa(bound.max+1), false)
	}
	for _, bound := range []struct{ name, min, max string }{
		{"CACHE_TTL_JITTER", "0s", "1m0s"},
		{"DB_CONN_MAX_LIFETIME", "1s", "1h0m0s"},
		{"HTTP_REQUEST_TIMEOUT", "1ms", "30s"},
		{"IDEMPOTENCY_TTL", "1m0s", "168h0m0s"},
		{"INVENTORY_BREAKER_INTERVAL", "1ms", "1h0m0s"},
		{"INVENTORY_BREAKER_OPEN_TIMEOUT", "1ms", "1m0s"},
		{"INVENTORY_CALL_TIMEOUT", "1ms", "10s"},
		{"INVENTORY_GETSTOCK_ATTEMPT_TIMEOUT", "0s", "10s"},
		{"INVENTORY_GRPC_IDLE_TIMEOUT", "0s", "24h0m0s"},
		{"INVENTORY_RETRY_BACKOFF", "0s", "1s"},
		{"INVENTORY_RETRY_MIN_BUDGET", "1ms", "1s"},
		{"ORDER_CLEANUP_TIMEOUT", "1ms", "5s"},
		{"ORDER_PUBLISH_TIMEOUT", "1ms", "10s"},
	} {
		min, err := time.ParseDuration(bound.min)
		if err != nil {
			t.Fatal(err)
		}
		max, err := time.ParseDuration(bound.max)
		if err != nil {
			t.Fatal(err)
		}
		checkRange(bound.name, (min - time.Nanosecond).String(), false)
		checkRange(bound.name, min.String(), true)
		checkRange(bound.name, max.String(), true)
		checkRange(bound.name, (max + time.Nanosecond).String(), false)
	}
}
