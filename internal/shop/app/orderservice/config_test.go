// 本文件：验证订单服务的治理参数和时间预算关系。
//
// 使用内存环境变量，不修改进程环境，也不会连接外部服务。
//
// 主要内容：
//   - TestConfigBudgets：检查默认值、无效重试次数和上下游预算关系。
//   - TestInventoryIdleConfig：验证空闲回收配置的默认值、关闭方式与输入边界。
//
// 阅读建议：单个时长合法不代表组合合法。下游必须比上游更早结束。

package orderservice

import (
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestConfigBudgets 每次只覆盖一个变量，便于知道是哪条配置规则拒绝了启动。
func TestConfigBudgets(t *testing.T) {
	for _, testcase := range []struct {
		name   string            // 描述要检查的配置边界。
		values map[string]string // 覆盖默认配置的变量。
		valid  bool              // 是否允许启动。
	}{
		{"缺少 Redis 密码", map[string]string{"REDIS_PASSWORD": ""}, false}, // 不能悄悄退回匿名连接。
		{"默认", nil, true}, // 默认预算应能完整覆盖下游和收尾。
		{"下游长于上游", map[string]string{"INVENTORY_CALL_TIMEOUT": "4s"}, false},                                                                                                     // 避免上游走了下游仍在执行。
		{"消息预算可独立配置", map[string]string{"ORDER_PUBLISH_TIMEOUT": "5s", "ORDER_CLEANUP_TIMEOUT": "2s"}, true},                                                                     // 消息确认不再被 Redis 收尾预算提前打断。
		{"消息预算有上限", map[string]string{"ORDER_PUBLISH_TIMEOUT": "11s"}, false},                                                                                                    // 防止发送无限占住处理资源。
		{"收尾不够", map[string]string{"ORDER_CLEANUP_TIMEOUT": "500ms"}, false},                                                                                                     // 补偿要容得下一次库存调用。
		{"最大重试", map[string]string{"INVENTORY_GETSTOCK_MAX_RETRIES": "10"}, true},                                                                                                // 实验允许放大，但总时限仍生效。
		{"重试过多", map[string]string{"INVENTORY_GETSTOCK_MAX_RETRIES": "11"}, false},                                                                                               // 和通用重试器的硬上限一致。
		{"负重试", map[string]string{"INVENTORY_GETSTOCK_MAX_RETRIES": "-1"}, false},                                                                                                // 负数没有可执行含义。
		{"继承总时限", map[string]string{"INVENTORY_GETSTOCK_ATTEMPT_TIMEOUT": "0s"}, true},                                                                                           // 默认不改变以往行为。
		{"短单次时限", map[string]string{"INVENTORY_GETSTOCK_ATTEMPT_TIMEOUT": "100ms"}, true},                                                                                        // 单次失败后仍留时间给下一次。
		{"单次等于总时限", map[string]string{"INVENTORY_GETSTOCK_ATTEMPT_TIMEOUT": "800ms"}, true},                                                                                      // 允许等号，但超时后没有重试余量。
		{"单次超过总时限", map[string]string{"INVENTORY_GETSTOCK_ATTEMPT_TIMEOUT": "801ms"}, false},                                                                                     // 拒绝看似有效却无法兑现的配置。
		{"负单次时限", map[string]string{"INVENTORY_GETSTOCK_ATTEMPT_TIMEOUT": "-1ms"}, false},                                                                                        // 只有零能表达沿用总时限。
		{"可关闭防护", map[string]string{"CACHE_SINGLEFLIGHT": "false", "CACHE_TTL_JITTER": "0s", "INVENTORY_BREAKER_ENABLED": "false", "INVENTORY_GETSTOCK_MAX_RETRIES": "0"}, true}, // 明确提供对比用的正常配置。
		{"错误失败率", map[string]string{"INVENTORY_BREAKER_FAILURE_PERCENT": "101"}, false},                                                                                          // 百分比不超过一百。
		{"空 broker 项", map[string]string{"KAFKA_BROKERS": "127.0.0.1:9092,"}, false},                                                                                             // 末尾逗号不能生成一个空地址。
		{"非整秒幂等期限", map[string]string{"IDEMPOTENCY_TTL": "60.5s"}, false},                                                                                                        // SET NX EX 使用整秒期限。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			reader := config.NewReader(func(key string) (string, bool) {
				if value, exists := testcase.values[key]; exists {
					return value, true
				}
				if key == "REDIS_PASSWORD" {
					return "demo-redis-test-only", true
				}
				if key == "MYSQL_DSN" {
					return "app:demo-only@tcp(127.0.0.1:3306)/shop?parseTime=true", true
				}
				value, exists := testcase.values[key]
				return value, exists
			})
			_, err := loadConfig(reader)
			if (err == nil) != testcase.valid {
				t.Fatalf("valid=%t err=%v", testcase.valid, err)
			}
		})
	}
}

// TestInventoryIdleConfig 保留正常的 30 分钟默认值，只在实验时允许缩短，不把修复变成关闭空闲回收。
func TestInventoryIdleConfig(t *testing.T) {
	for _, testcase := range []struct {
		name  string        // 用例说明。
		raw   string        // 模拟操作者输入的时长；空值使用默认值。
		want  time.Duration // 成功时应交给连接管理器的时间。
		valid bool          // 格式和范围是否允许启动。
	}{
		{"默认", "", 30 * time.Minute, true},      // 与 grpc-go 原默认行为保持一致。
		{"快速实验", "10s", 10 * time.Second, true}, // 能在本机快速观察空闲重建。
		{"显式关闭", "0s", 0, true},                 // 保留 grpc-go 用零关闭回收的语义。
		{"负数", "-1s", 0, false},                 // 负数不是有效的等待时间。
		{"过长", "25h", 0, false},                 // 拒绝超出一天的误填参数。
		{"错误单位", "10minutes", 0, false},         // 不能把无法解析的输入静默当成默认值。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			reader := config.NewReader(func(key string) (string, bool) {
				if key == "REDIS_PASSWORD" {
					return "demo-redis-test-only", true
				}
				if key == "MYSQL_DSN" {
					return "app:demo-only@tcp(127.0.0.1:3306)/shop?parseTime=true", true
				}
				return testcase.raw, key == "INVENTORY_GRPC_IDLE_TIMEOUT" && testcase.raw != ""
			})
			settings, err := loadConfig(reader)
			if (err == nil) != testcase.valid || (err == nil && settings.InventoryIdleTimeout != testcase.want) {
				t.Fatalf("idle=%v err=%v", settings.InventoryIdleTimeout, err)
			}
		})
	}
}
