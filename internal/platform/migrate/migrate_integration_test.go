package migrate

import (
	"context"
	"database/sql"
	"embed"
	"os"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/go-sql-driver/mysql"
)

// embeddedProbe 是编译器嵌入的只读 SQL；声明后绝不赋值，实际验证 embed.FS 的迁移路径。
//
//go:embed testdata/*.sql
var embeddedProbe embed.FS

// integrationDatabase 连接专用测试库；缺少开关时跳过，避免普通 go test 访问网络。
// TEST_MIGRATE_MYSQL_DSN 必须指向可清空的独立测试库，不能指向 shop 或 doctor 业务库。
func integrationDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_MIGRATE_MYSQL_DSN")
	if dsn == "" {
		t.Skip("设置 TEST_MIGRATE_MYSQL_DSN 指向独立测试库后运行 go test ./internal/platform/migrate -run Integration -v")
	}
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil || parsed.DBName != "opsdoctor_migrate_test" {
		t.Fatal("迁移测试只允许使用 opsdoctor_migrate_test 专用库，拒绝清理其他数据库")
	}
	database, err := mysqlx.Open(t.Context(), mysqlx.Config{DSN: dsn, MaxOpenConns: 4, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	// DROP 仅作用于显式授权的测试库中的两个探针表；DDL 持有元数据锁，没有行索引访问。
	const dropHistorySQL = `DROP TABLE IF EXISTS schema_migrations`
	const dropProbeSQL = `DROP TABLE IF EXISTS migration_probe`
	for _, statement := range []string{dropProbeSQL, dropHistorySQL} {
		if _, err := database.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	return database
}

// TestApplyIntegration 验证两个并发启动者只记一次迁移，改写被拒绝，取消后锁仍能正确释放。
func TestApplyIntegration(t *testing.T) {
	database := integrationDatabase(t)
	// 20 秒只用于测试上界；命名锁会把两个启动者串起来，成功后都应返回。
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	var workers sync.WaitGroup     // 等待两个迁移调用退出，不让 goroutine 跑到测试结束之后。
	results := make(chan error, 2) // 两个工作者各写一次，主测试读两次；无需关闭，容量避免写入阻塞。
	for range 2 {
		workers.Go(func() {
			// ctx 超时会中断数据库操作；Wait 保证工作者在测试返回前全部退出。
			results <- Apply(ctx, database, embeddedProbe, "testdata")
		})
	}
	workers.Wait()
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	// countSQL 扫描很小的历史表，只读不加锁；预期只有一条主键记录。
	const countSQL = `SELECT COUNT(*) FROM schema_migrations`
	var count int
	if err := database.QueryRowContext(ctx, countSQL).Scan(&count); err != nil || count != 1 {
		t.Fatalf("记录数 %d：%v", count, err)
	}
	changed := fstest.MapFS{"testdata/0001_probe.sql": &fstest.MapFile{Data: []byte("SELECT 2")}}
	if err := Apply(ctx, database, changed, "testdata"); err == nil {
		t.Fatal("接受了改写的历史迁移")
	}
	verifyCancellation(t, database)
}

// verifyCancellation 持锁后取消调用方，再释放并重新获取，验证清理没有复用已取消的 context。
func verifyCancellation(t *testing.T, database *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	connection, err := database.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := closeConnection(connection); err != nil {
			t.Error(err)
		}
	}()
	name, err := acquire(ctx, connection)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	if err := release(connection, name); err != nil {
		t.Fatal(err)
	}
	// 使用另一个会话拿同一把锁；若释放失败，这里会因短预算失败。
	probeCtx, stop := context.WithTimeout(t.Context(), time.Second)
	defer stop()
	if err := Apply(probeCtx, database, embeddedProbe, "testdata"); err != nil {
		t.Fatal(err)
	}
}
