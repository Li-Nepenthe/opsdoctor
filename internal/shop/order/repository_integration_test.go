// 本文件：在真实 MySQL 中验证订单保存、详情和分页。
//
// 只允许 opsdoctor_order_test 专用库。测试不删除或修改 shop 中已有的两百万条订单。
//
// 主要内容：
//   - integrationDatabase：检查库名并准备独立测试表。
//   - TestIntegrationOrderQueries：保存订单，核对详情、用户隔离和稳定分页。
//   - TestIntegrationOrderRejectedBeforeCommit：验证数据库拒绝时能明确说明订单未提交。
//
// 阅读建议：测试故意使用相同创建时间，确认列表仍按订单编号稳定排序。

package order

import (
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/migrations"
	"github.com/go-sql-driver/mysql"
)

// integrationDatabase 连接前检查数据库名称，清理只发生在明确的测试库。
func integrationDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_ORDER_MYSQL_DSN")
	if dsn == "" {
		t.Skip("设置 TEST_ORDER_MYSQL_DSN 为 opsdoctor_order_test 后运行订单集成测试")
	}
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil || parsed.DBName != "opsdoctor_order_test" {
		t.Fatal("只允许 opsdoctor_order_test，未连接或清理任何库")
	}
	database, err := mysqlx.Open(t.Context(), mysqlx.Config{DSN: dsn, MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := migrations.Apply(t.Context(), database); err != nil {
		t.Fatal(err)
	}
	// 固定的清理语句只在上述白名单验证后运行，不允许环境变量选择别的表。
	if _, err := database.ExecContext(t.Context(), "DELETE FROM orders"); err != nil {
		t.Fatal(err)
	}
	return database
}

// TestIntegrationOrderQueries 从真实写入结果查询，避免只验证一套手工准备的数据。
func TestIntegrationOrderQueries(t *testing.T) {
	repository := NewMySQLRepository(integrationDatabase(t))
	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	var saved []Order
	for range 3 {
		value, err := repository.Create(t.Context(), Order{UserID: 1, SKUID: 2, Quantity: 3, AmountCents: 600, Status: "created", CreatedAt: createdAt})
		if err != nil {
			t.Fatal(err)
		}
		saved = append(saved, value)
	}
	// 插入另一个用户，检验列表不会漏掉 WHERE user_id 的限制。
	if _, err := repository.Create(t.Context(), Order{UserID: 2, SKUID: 2, Quantity: 1, AmountCents: 200, Status: "created", CreatedAt: createdAt}); err != nil {
		t.Fatal(err)
	}
	detail, err := repository.Get(t.Context(), saved[0].OrderID)
	if err != nil || detail.OrderID != saved[0].OrderID || detail.Quantity != 3 || detail.AmountCents != 600 || !detail.CreatedAt.Equal(createdAt) {
		t.Fatalf("详情不一致：%+v %v", detail, err)
	}
	first, err := repository.List(t.Context(), 1, 1, 2)
	if err != nil || len(first.Items) != 2 || first.Items[0].OrderID != saved[2].OrderID || first.Items[1].OrderID != saved[1].OrderID {
		t.Fatalf("第一页顺序：%+v %v", first, err)
	}
	second, err := repository.List(t.Context(), 1, 2, 2)
	if err != nil || len(second.Items) != 1 || second.Items[0].OrderID != saved[0].OrderID {
		t.Fatalf("第二页：%+v %v", second, err)
	}
	empty, err := repository.List(t.Context(), 99, 1, 20)
	if err != nil || empty.Items == nil || len(empty.Items) != 0 {
		t.Fatalf("空列表：%+v %v", empty, err)
	}
	if _, err := repository.Get(t.Context(), 9223372036854775807); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在的订单：%v", err)
	}
}

// TestIntegrationOrderRejectedBeforeCommit 让 CHECK 约束拒绝负数量，此时还没有发送 COMMIT。
func TestIntegrationOrderRejectedBeforeCommit(t *testing.T) {
	database := integrationDatabase(t)
	repository := NewMySQLRepository(database)
	_, err := repository.Create(t.Context(), Order{UserID: 1, SKUID: 1, Quantity: -1, AmountCents: 100, Status: "created", CreatedAt: time.Now().UTC()})
	var failure *PersistenceError
	if !errors.As(err, &failure) || failure.MayHaveCommitted {
		t.Fatalf("事务失败分类：%v", err)
	}
	var count int
	if err := database.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM orders").Scan(&count); err != nil || count != 0 {
		t.Fatalf("失败后仍留下订单：%d %v", count, err)
	}
}
