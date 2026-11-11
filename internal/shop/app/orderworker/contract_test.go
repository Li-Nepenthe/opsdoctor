// 本文件：给消费者入口显式传入非法配置，观察它在连接数据库或 Kafka 之前返回的错误。

package orderworker

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestWorkerEnvironmentContract 从启动入口确认配置错误的顺序，并检查消息时限两端的输入。
func TestWorkerEnvironmentContract(t *testing.T) {
	for _, key := range strings.Fields("MYSQL_DSN DB_MAX_OPEN_CONNS DB_MAX_IDLE_CONNS DB_CONN_MAX_LIFETIME ADMIN_ADDR KAFKA_BROKERS WORKER_MESSAGE_TIMEOUT") {
		t.Setenv(key, "")
	}
	t.Setenv("DB_MAX_OPEN_CONNS", "20")
	t.Setenv("WORKER_MESSAGE_TIMEOUT", "invalid")
	err := Run(t.Context())
	if err == nil {
		t.Fatal("消费者接受了缺失的配置")
	}
	// 必填项、数据库池和消息预算的边界在任何启动副作用前就能观察到。
	want := strings.Join([]string{"环境变量 MYSQL_DSN 不能为空", "环境变量 DB_MAX_IDLE_CONNS 不能为空", "环境变量 DB_CONN_MAX_LIFETIME 不能为空", "环境变量 ADMIN_ADDR 不能为空", "环境变量 KAFKA_BROKERS 不能为空", "环境变量 WORKER_MESSAGE_TIMEOUT 必须是 1s 到 30s 之间的时长（例如 5s）"}, "\n")
	if err.Error() != want {
		t.Fatalf("启动校验变化：\n%v", err)
	}
	// 故意继续缺少 MYSQL_DSN，让通过时长校验的用例也在联网之前结束。
	for _, test := range []struct {
		raw   string
		valid bool
	}{{"999999999ns", false}, {"1s", true}, {"30s", true}, {"30000000001ns", false}} {
		t.Setenv("WORKER_MESSAGE_TIMEOUT", test.raw)
		err := Run(t.Context())
		if err == nil || strings.Contains(err.Error(), "WORKER_MESSAGE_TIMEOUT") == test.valid {
			t.Fatalf("消息时限 %s：%v", test.raw, err)
		}
	}
}

// TestWorkerConfigDefaults 只提供数据库连接串，使三项消费者配置都取到启动时的默认值。
func TestWorkerConfigDefaults(t *testing.T) {
	reader := config.NewReader(func(name string) (string, bool) {
		return "test-only", name == "MYSQL_DSN"
	})
	settings, err := loadConfig(reader)
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprintf("ADMIN_ADDR\t%s\nKAFKA_BROKERS\t%v\nWORKER_MESSAGE_TIMEOUT\t%s\n",
		settings.AdminAddress, settings.KafkaBrokers, settings.MessageTimeout)
	want, err := os.ReadFile("testdata/environment.golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("消费者默认配置发生变化：\n得到\n%s期望\n%s", got, want)
	}
}

// TestWorkerConfigErrorOrder 地址列表要等所有变量读好才会展开，因此端口的读取错误先于 Kafka 格式错误返回。
func TestWorkerConfigErrorOrder(t *testing.T) {
	values := map[string]string{"MYSQL_DSN": "test-only", "ADMIN_ADDR": " ", "KAFKA_BROKERS": "broker:9092,"}
	reader := config.NewReader(func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	})
	if _, err := loadConfig(reader); err == nil || err.Error() != "环境变量 ADMIN_ADDR 不能为空" {
		t.Fatalf("管理地址为空时应先报告读取错误：%v", err)
	}
	delete(values, "ADMIN_ADDR")
	if _, err := loadConfig(reader); err == nil || err.Error() != "KAFKA_BROKERS 必须是逗号分隔的主机:端口" {
		t.Fatalf("管理地址有效后应报告 Kafka 列表中的空项：%v", err)
	}
}

// TestWorkerConfigOverrides 直接核对装配后的地址和消息时限，覆盖时限两端及其相邻的越界值。
func TestWorkerConfigOverrides(t *testing.T) {
	const timeoutError = "环境变量 WORKER_MESSAGE_TIMEOUT 必须是 1s 到 30s 之间的时长（例如 5s）"
	for _, test := range []struct {
		raw     string
		want    time.Duration
		wantErr string
	}{
		{"999999999ns", 0, timeoutError},
		{"1s", time.Second, ""},
		{"30s", 30 * time.Second, ""},
		{"30000000001ns", 0, timeoutError},
	} {
		t.Run(test.raw, func(t *testing.T) {
			values := map[string]string{
				"MYSQL_DSN":              "test-only",
				"ADMIN_ADDR":             " 127.0.0.1:7600 ",
				"KAFKA_BROKERS":          " broker:29092, [::1]:39092 ",
				"WORKER_MESSAGE_TIMEOUT": test.raw,
			}
			reader := config.NewReader(func(name string) (string, bool) {
				value, ok := values[name]
				return value, ok
			})
			settings, err := loadConfig(reader)
			if test.wantErr != "" {
				if err == nil || err.Error() != test.wantErr {
					t.Fatalf("消息时限 %s：%v", test.raw, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if settings.AdminAddress != "127.0.0.1:7600" || !slices.Equal(settings.KafkaBrokers, []string{"broker:29092", "[::1]:39092"}) || settings.MessageTimeout != test.want {
				t.Fatalf("实际配置：管理地址 %q，Kafka %v，消息时限 %s", settings.AdminAddress, settings.KafkaBrokers, settings.MessageTimeout)
			}
		})
	}
}
