// 本文件：在真实 MySQL 中验证销量去重与事务回滚。
//
// 只允许 opsdoctor_sales_test 专用库，测试准备不会接触 shop 的真实造数结果。
//
// 主要内容：
//   - integrationDatabase：检查库名，应用迁移，再清理测试数据。
//   - TestIntegrationDuplicateSales：同一订单并发送达，只累加一次。
//   - TestIntegrationSalesRollback：加销量失败后，去重记录也必须撤回。
//   - TestIntegrationCheckSales：按件数核对订单与销量，而不是按订单笔数。
//
// 阅读建议：先看测试库白名单，再看溢出实验为什么能证明事务没有只成功一半。

package sales

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/events"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/migrations"
	"github.com/go-sql-driver/mysql"
)

// integrationDatabase 在连接前校验专用库名，拒绝把清理语句发给业务库。
func integrationDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_SALES_MYSQL_DSN")
	if dsn == "" {
		t.Skip("设置 TEST_SALES_MYSQL_DSN 为 opsdoctor_sales_test 后运行真实销量测试")
	}
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil || parsed.DBName != "opsdoctor_sales_test" {
		t.Fatal("只允许 opsdoctor_sales_test，未连接或清理任何库")
	}
	database, err := mysqlx.Open(t.Context(), mysqlx.Config{DSN: dsn, MaxOpenConns: 10, MaxIdleConns: 5, ConnMaxLifetime: time.Minute})
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
	// 这些固定表名来自测试本身，绝不接受环境变量拼接表名。
	for _, statement := range []string{"DELETE FROM processed_events", "DELETE FROM product_sales", "DELETE FROM orders"} {
		if _, err := database.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	return database
}

// TestIntegrationDuplicateSales 用真实唯一键让二十条相同消息竞争，同一订单只能记一次账。
func TestIntegrationDuplicateSales(t *testing.T) {
	database := integrationDatabase(t)
	repository := NewMySQLRepository(database)
	event := events.OrderCreated{OrderID: 1, UserID: 1, SKUID: 1, Quantity: 3, AmountCents: 300, CreatedAt: time.Now().UTC()}
	results := make(chan error, 20)
	start := make(chan struct{})
	for range 20 {
		go func() { <-start; results <- repository.Apply(t.Context(), event) }()
	}
	close(start)
	// 收齐结果后再读销量，避免把仍在处理中的消息误当成漏统计。
	for range 20 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	var sold, processed int64
	if err := database.QueryRowContext(t.Context(), "SELECT sold FROM product_sales WHERE sku_id = 1").Scan(&sold); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM processed_events WHERE order_id = 1").Scan(&processed); err != nil {
		t.Fatal(err)
	}
	if sold != 3 || processed != 1 {
		t.Fatalf("销量=%d 去重记录=%d", sold, processed)
	}
	t.Logf("同一订单并发送达 20 次：销量=%d，去重记录=%d", sold, processed)
}

// TestIntegrationSalesRollback 故意让 BIGINT 加法溢出，验证第一条 INSERT 也被回滚。
func TestIntegrationSalesRollback(t *testing.T) {
	database := integrationDatabase(t)
	if _, err := database.ExecContext(t.Context(), "INSERT INTO product_sales (sku_id, sold) VALUES (1, 9223372036854775807)"); err != nil {
		t.Fatal(err)
	}
	repository := NewMySQLRepository(database)
	event := events.OrderCreated{OrderID: 1, SKUID: 1, Quantity: 1}
	if err := repository.Apply(t.Context(), event); err == nil {
		t.Fatal("溢出没有失败")
	}
	var count int
	if err := database.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM processed_events").Scan(&count); err != nil || count != 0 {
		t.Fatalf("失败事务留下去重记录：%d %v", count, err)
	}
	// 把测试数据恢复到合法范围后重放，应该还能成功，而不是被残留凭据挡住。
	if _, err := database.ExecContext(t.Context(), "UPDATE product_sales SET sold = 0 WHERE sku_id = 1"); err != nil {
		t.Fatal(err)
	}
	if err := repository.Apply(t.Context(), event); err != nil {
		t.Fatal(err)
	}
	var sold int64
	if err := database.QueryRowContext(t.Context(), "SELECT sold FROM product_sales WHERE sku_id = 1").Scan(&sold); err != nil || sold != 1 {
		t.Fatalf("重放结果：%d %v", sold, err)
	}
}

// TestIntegrationCheckSales 用一笔三件的订单证明检查的是件数，空表也应返回零。
func TestIntegrationCheckSales(t *testing.T) {
	database := integrationDatabase(t)
	repository := NewMySQLRepository(database)
	before, err := repository.Check(t.Context())
	if err != nil || before != (Summary{}) {
		t.Fatalf("空表核对：%+v %v", before, err)
	}
	if _, err := database.ExecContext(t.Context(), "INSERT INTO orders (user_id, sku_id, quantity, amount_cents, status, created_at) VALUES (1, 1, 3, 300, 'created', UTC_TIMESTAMP(6))"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(t.Context(), "INSERT INTO product_sales (sku_id, sold) VALUES (1, 2)"); err != nil {
		t.Fatal(err)
	}
	// 一笔订单买三件，目前只统计两件，所以差值应为一件。
	after, err := repository.Check(t.Context())
	if err != nil || after.Ordered != 3 || after.Sold != 2 || after.Gap != 1 {
		t.Fatalf("核对结果：%+v %v", after, err)
	}
}
