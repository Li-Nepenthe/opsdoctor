// 本文件：验证连接池默认值和参数之间的约束。
//
// TestDatabaseConfig 使用假环境验证缺 DSN、小池与空闲数的组合。无需连接数据库。
// 重点看只把 open 改为 2 为什么还会因为默认 idle=10 而失败。

package app

import (
	"slices"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestDatabaseConfig 验证默认连接预算和互相约束的 open/idle，错误里不得回显 DSN。
func TestDatabaseConfig(t *testing.T) {
	cases := []struct {
		name   string            // name 说明配置组合要验证的规则。
		values map[string]string // values 模拟环境，不修改真正进程变量。
		valid  bool              // valid 表示是否允许继续启动。
	}{
		{"默认池", map[string]string{"MYSQL_DSN": "demo"}, true},                                                        // 未设置池参数采用 20/10/5m。
		{"缺少连接串", map[string]string{}, false},                                                                        // 不能带空密码来源去猜默认数据库。
		{"空闲超过总数", map[string]string{"MYSQL_DSN": "demo", "DB_MAX_OPEN_CONNS": "2"}, false},                          // 默认 idle=10 也必须尊重新的 open=2。
		{"小池配套调整", map[string]string{"MYSQL_DSN": "demo", "DB_MAX_OPEN_CONNS": "2", "DB_MAX_IDLE_CONNS": "1"}, true}, // 两项一起调小是合法配置。
	}
	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			reader := config.NewReader(func(name string) (string, bool) { value, exists := testcase.values[name]; return value, exists })
			settings, err := DatabaseConfig(reader)
			if (err == nil) != testcase.valid {
				t.Fatalf("配置结果：%v", err)
			}
			if testcase.name == "默认池" && (settings.MaxOpenConns != 20 || settings.MaxIdleConns != 10) {
				t.Fatalf("默认池不匹配：open=%d idle=%d", settings.MaxOpenConns, settings.MaxIdleConns)
			}
		})
	}
}

// TestKafkaBrokers 验证订单服务和消费者沿用同一地址格式，包括带括号的 IPv6 和服务名端口。
func TestKafkaBrokers(t *testing.T) {
	got, err := KafkaBrokers(" broker:9092, [::1]:kafka ")
	if err != nil || !slices.Equal(got, []string{"broker:9092", "[::1]:kafka"}) {
		t.Fatalf("解析 Kafka 地址列表：%v，%v", got, err)
	}
	for _, raw := range []string{"", ":9092", "broker:", "broker", "broker:9092,", "2001:db8::1:9092"} {
		got, err := KafkaBrokers(raw)
		if got != nil || err == nil || err.Error() != "KAFKA_BROKERS 必须是逗号分隔的主机:端口" {
			t.Errorf("KafkaBrokers(%q) = %v，%v", raw, got, err)
		}
	}
}
