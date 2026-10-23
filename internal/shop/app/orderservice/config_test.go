// 本文件：验证订单服务的治理参数和时间预算关系。
//
// 使用内存环境变量，不修改进程环境，也不会连接外部服务。
//
// 主要内容：
//   - TestConfigBudgets：检查默认值、无效重试次数和上下游预算关系。
//
// 阅读建议：单个时长合法不代表组合合法。下游必须比上游更早结束。

package orderservice

import (
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestConfigBudgets 每次只覆盖一个变量，便于知道是哪条配置规则拒绝了启动。
func TestConfigBudgets(t *testing.T) {
	for _, testcase := range []struct {
		name   string            // 描述要检查的配置边界。
		values map[string]string // 覆盖默认配置的变量。
		valid  bool              // 是否允许启动。
	}{
		{"默认", nil, true}, // 默认预算应能完整覆盖下游和收尾。
		{"下游长于上游", map[string]string{"INVENTORY_CALL_TIMEOUT": "4s"}, false},                                                                                                     // 避免上游走了下游仍在执行。
		{"收尾不够", map[string]string{"ORDER_CLEANUP_TIMEOUT": "500ms"}, false},                                                                                                     // 补偿要容得下一次库存调用。
		{"重试过多", map[string]string{"INVENTORY_GETSTOCK_MAX_RETRIES": "3"}, false},                                                                                                // 计划书最多两次重试。
		{"可关闭防护", map[string]string{"CACHE_SINGLEFLIGHT": "false", "CACHE_TTL_JITTER": "0s", "INVENTORY_BREAKER_ENABLED": "false", "INVENTORY_GETSTOCK_MAX_RETRIES": "0"}, true}, // 明确提供对比用的正常配置。
		{"错误失败率", map[string]string{"INVENTORY_BREAKER_FAILURE_PERCENT": "101"}, false},                                                                                          // 百分比不超过一百。
		{"空 broker 项", map[string]string{"KAFKA_BROKERS": "127.0.0.1:9092,"}, false},                                                                                             // 末尾逗号不能生成一个空地址。
		{"非整秒幂等期限", map[string]string{"IDEMPOTENCY_TTL": "60.5s"}, false},                                                                                                        // SET NX EX 使用整秒期限。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			reader := config.NewReader(func(key string) (string, bool) {
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
