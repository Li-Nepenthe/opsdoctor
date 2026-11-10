// 本文件：在专用数据库验证迁移和命名锁。
//
// 测试会清理探针表，因此先检查 DSN 的库名白名单。
// TestApplyIntegration 使用 testdata 的 SQL 验证重复运行与历史保护。先读准备步骤，再看真实连接上的断言。
//
// 主要内容：
//   - integrationDatabase：连接专用测试库。缺少开关时跳过，避免普通 go test 访问网络。
//   - TestApplyIntegration：验证两个并发启动者只记一次迁移，改写被拒绝，取消后锁仍能正确释放。
//   - TestApplyRecoveryIntegration：验证执行中取消或 SQL 失败只保留成功历史，修正后能重新拿锁并继续。
//   - verifyCancellation：持锁后取消调用方，再释放并重新获取，验证清理没有复用已取消的 context。

package migrate

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"os"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/go-sql-driver/mysql"
)

// embeddedProbe 是编译器嵌入的只读 SQL。声明后绝不赋值，实际验证 embed.FS 的迁移路径。
//
//go:embed testdata/*.sql
var embeddedProbe embed.FS

// integrationDatabase 连接专用测试库。缺少开关时跳过，避免普通 go test 访问网络。
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
	// 每次测试从空表状态开始，只删除上面已确认的专用库中的这两张测试表。
	// DROP 删除整张表，需要元数据锁。它不按索引逐行查找，也不是逐行加锁的 DELETE。
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
	// 20 秒只用于测试上界。命名锁会把两个启动者串起来，成功后都应返回。
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	var workers sync.WaitGroup     // 等待两个迁移调用退出，不让 goroutine 跑到测试结束之后。
	results := make(chan error, 2) // 两个工作者各写一次，主测试读两次；无需关闭，容量避免写入阻塞。
	for range 2 {
		workers.Go(func() {
			// ctx 超时会中断数据库操作。Wait 保证工作者在测试返回前全部退出。
			results <- Apply(ctx, database, embeddedProbe, "testdata")
		})
	}
	workers.Wait()
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	// countSQL 统计很小的历史表，InnoDB 会选择可用索引来计数。普通快照读不加行锁。
	// 两个调用都成功了，但它们执行的是同一份迁移，所以历史里必须只有一条记录。
	const countSQL = `SELECT COUNT(*) FROM schema_migrations`
	var count int
	if err := database.QueryRowContext(ctx, countSQL).Scan(&count); err != nil || count != 1 {
		t.Fatalf("记录数 %d：%v", count, err)
	}
	// 保留文件名却替换 SQL，模拟“修改已执行迁移”。必须由校验和检查拒绝。
	changed := fstest.MapFS{"testdata/0001_probe.sql": &fstest.MapFile{Data: []byte("SELECT 2")}}
	if err := Apply(ctx, database, changed, "testdata"); err == nil {
		t.Fatal("接受了改写的历史迁移")
	}
	verifyCancellation(t, database)
}

// TestApplyRecoveryIntegration 验证执行中取消或 SQL 失败只保留成功历史，修正后能重新拿锁并继续。
func TestApplyRecoveryIntegration(t *testing.T) {
	for _, test := range []struct {
		name, middle  string // middle 是故意阻塞或写错的第 2 个迁移。
		cancelRunning bool   // 取消用例必须先看到 SQL 真正在执行，不能用固定延时猜测。
	}{
		{"cancel_during_migration", "SELECT SLEEP(5)", true},
		{"repair_failed_migration", "INSER INTO migration_probe VALUES (1)", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			database := integrationDatabase(t)
			first, err := embeddedProbe.ReadFile("testdata/0001_probe.sql")
			if err != nil {
				t.Fatal(err)
			}
			// 复用原来的建表迁移；后两个文件只写内存，不改生产 SQL。
			files := fstest.MapFS{
				"0001_probe.sql":  {Data: first},
				"0002_middle.sql": {Data: []byte(test.middle)},
				"0003_finish.sql": {Data: []byte("INSERT INTO migration_probe VALUES (2)")},
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- Apply(ctx, database, files, ".") }()
			var running int
			var observeErr error
			if test.cancelRunning {
				// 同账号可以查看自己的连接；精确匹配 SQL 并排除观察连接，无需 PROCESS 权限。
				const runningSQL = `SELECT COUNT(*) FROM information_schema.PROCESSLIST
					WHERE ID <> CONNECTION_ID() AND DB = DATABASE() AND INFO = ?`
				deadline := time.Now().Add(4 * time.Second)
				for time.Now().Before(deadline) {
					observeErr = database.QueryRowContext(t.Context(), runningSQL, test.middle).Scan(&running)
					if observeErr != nil || running > 0 {
						break
					}
					time.Sleep(20 * time.Millisecond)
				}
				cancel()
			}
			// 先等 Apply 完整退出再断言，观察失败时也不能把后台连接留给下一个用例。
			err = <-done
			if test.cancelRunning && (running == 0 || observeErr != nil) {
				t.Fatalf("未观察到第 2 个迁移正在执行：%v", observeErr)
			}
			if err == nil || test.cancelRunning && !errors.Is(err, context.Canceled) {
				t.Fatalf("未返回预期的 SQL 失败或 context.Canceled：%v", err)
			}
			// 同时查历史和实际写入，避免只证明没有记账，却漏掉后续 SQL 已经执行的情况。
			const stateSQL = `SELECT COUNT(*), COALESCE(MAX(version), ''),
				(SELECT COUNT(*) FROM migration_probe) FROM schema_migrations`
			var count, marks int
			var latest string
			if err := database.QueryRowContext(t.Context(), stateSQL).Scan(&count, &latest, &marks); err != nil || count != 1 || latest != "0001_probe.sql" || marks != 0 {
				t.Fatalf("中断后历史=%d，最后文件=%s，写入=%d：%v", count, latest, marks, err)
			}
			files["0002_middle.sql"].Data = []byte("INSERT INTO migration_probe VALUES (1)")
			// 新的调用预算也验证命名锁已经释放，不需要人工清理状态就能继续。
			retryCtx, stop := context.WithTimeout(t.Context(), 10*time.Second)
			defer stop()
			if err := Apply(retryCtx, database, files, "."); err != nil {
				t.Fatal(err)
			}
			if err := database.QueryRowContext(retryCtx, stateSQL).Scan(&count, &latest, &marks); err != nil || count != 3 || latest != "0003_finish.sql" || marks != 2 {
				t.Fatalf("修正后历史=%d，最后文件=%s，写入=%d：%v", count, latest, marks, err)
			}
		})
	}
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
	// 使用另一个会话拿同一把锁。若释放失败，这里会因短预算失败。
	probeCtx, stop := context.WithTimeout(t.Context(), time.Second)
	defer stop()
	if err := Apply(probeCtx, database, embeddedProbe, "testdata"); err != nil {
		t.Fatal(err)
	}
}
