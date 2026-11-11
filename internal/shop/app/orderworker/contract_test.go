// 本文件：给消费者入口显式传入非法配置，观察它在连接数据库或 Kafka 之前返回的错误。

package orderworker

import (
	"strings"
	"testing"
)

// TestWorkerEnvironmentContract 单个启动阶段要一次列出所有配置问题；借这个出口观察未抽出的配置读取过程。
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
