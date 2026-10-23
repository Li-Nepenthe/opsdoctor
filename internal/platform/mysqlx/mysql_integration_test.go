// 本文件：用真实 MySQL 检查连接池和会话时区。
//
// 只有设置 TEST_MYSQL_DSN 才运行。普通单元测试可以脱离 Docker。
// TestOpenIntegration 通过实际查询确认 UTC 生效，不能仅凭 Go 配置对象里写了时区就认定数据库已改变。
//
// 主要内容：
//   - TestOpenIntegration：验证真实会话使用 UTC。没有专门的测试 DSN 时明确跳过，普通测试不联网。

package mysqlx

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestOpenIntegration 验证真实会话使用 UTC。没有专门的测试 DSN 时明确跳过，普通测试不联网。
func TestOpenIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("设置 TEST_MYSQL_DSN 后运行 go test ./internal/platform/mysqlx -run Integration -v")
	}
	// 10 秒是本测试的整体预算，避免依赖异常时无限等待。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	database, err := Open(ctx, Config{DSN: dsn, MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	// sessionTimeZoneSQL 查询这条连接实际使用的时区，验证驱动确实把 UTC 设置发给了 MySQL。
	// 会话变量不存放在业务表里，这条语句不查表，因此不需要表索引，也不会给数据行加锁。
	const sessionTimeZoneSQL = `SELECT @@session.time_zone`
	var zone string
	if err := database.QueryRowContext(ctx, sessionTimeZoneSQL).Scan(&zone); err != nil {
		t.Fatal(err)
	}
	if zone != "+00:00" {
		t.Fatalf("会话时区：%s", zone)
	}
}
