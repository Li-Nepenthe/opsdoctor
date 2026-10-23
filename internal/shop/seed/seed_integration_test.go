// 本文件：用独立 MySQL 库验证续跑、跳过与回滚。
//
// seedDatabase 先校验库名再清理，和库存测试用不同库，防止并行包互相干扰。先读正常续跑与库存保留，再看故意让订单主键冲突时，前面已插入的商品为何必须回滚。
//
// 主要内容：
//   - seedDatabase：仅在专用库清理五张业务表。使用独立库避免与库存包并行测试时互相删数据。
//   - TestIntegrationSeed：验证从已提交批次继续、重复跳过、保持用户改过的库存及五表数量一致。
//   - TestIntegrationSeedRollback：故意制造第三张表的主键冲突，证明前面两张表不会留下半批数据。

package seed

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/migrations"
	"github.com/go-sql-driver/mysql"
)

// seedDatabase 仅在专用库清理五张业务表。使用独立库避免与库存包并行测试时互相删数据。
func seedDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_SEED_MYSQL_DSN")
	if dsn == "" {
		t.Skip("设置 TEST_SEED_MYSQL_DSN 为 opsdoctor_seed_test 专用库以开启造数集成测试，见 README")
	}
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil || parsed.DBName != "opsdoctor_seed_test" {
		t.Fatal("仅允许 opsdoctor_seed_test，未执行清理")
	}
	database, err := mysqlx.Open(t.Context(), mysqlx.Config{DSN: dsn, MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute})
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
	// 表名全部来自固定白名单，DELETE 全表只用于这个可丢弃测试库。没有外键，顺序仍按依赖从后往前。
	for _, table := range []string{"processed_events", "product_sales", "orders", "stock", "products"} {
		if _, err := database.ExecContext(t.Context(), "DELETE FROM "+table); err != nil {
			t.Fatal(err)
		}
	}
	return database
}

// TestIntegrationSeed 验证从已提交批次继续、重复跳过、保持用户改过的库存及五表数量一致。
func TestIntegrationSeed(t *testing.T) {
	database := seedDatabase(t)
	// 先提交一个很小的完整批次，模拟上次初始化在批次边界中断。
	if err := insertBatch(t.Context(), database, 1, 2); err != nil {
		t.Fatal(err)
	}
	result, err := Run(t.Context(), database, true)
	if err != nil || result.Products != 100 || result.Orders != 400 || result.InsertedProducts != 98 {
		t.Fatalf("续跑：%+v %v", result, err)
	}
	// 唯一索引命中 SKU=1，只改这一行。重跑 seed 不能把它重置。
	if _, err := database.ExecContext(t.Context(), "UPDATE stock SET available=7 WHERE sku_id=1"); err != nil {
		t.Fatal(err)
	}
	result, err = Run(t.Context(), database, true)
	if err != nil || !result.Skipped || result.InsertedProducts != 0 {
		t.Fatalf("重跑：%+v %v", result, err)
	}
	var available int
	if err := database.QueryRowContext(t.Context(), "SELECT available FROM stock WHERE sku_id=1").Scan(&available); err != nil || available != 7 {
		t.Fatalf("重跑覆盖了库存：%d %v", available, err)
	}
	t.Logf("small：products=%d orders=%d；重跑 skipped=%t，已修改库存保持 %d", result.Products, result.Orders, result.Skipped, available)
}

// TestIntegrationSeedRollback 故意制造第三张表的主键冲突，证明前面两张表不会留下半批数据。
func TestIntegrationSeedRollback(t *testing.T) {
	database := seedDatabase(t)
	if err := insertBatch(t.Context(), database, 1, 2); err != nil {
		t.Fatal(err)
	}
	// 在主键 9 预放一条订单，让 SKU=3 的事务在商品、库存写完后才失败。
	const collide = `INSERT INTO orders (id,user_id,sku_id,quantity,amount_cents,status,created_at) VALUES (9,1001,3,1,1999,'created',UTC_TIMESTAMP(6))`
	if _, err := database.ExecContext(t.Context(), collide); err != nil {
		t.Fatal(err)
	}
	if err := insertBatch(t.Context(), database, 3, 3); err == nil {
		t.Fatal("预期主键冲突")
	}
	var count int
	if err := database.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM products WHERE id=3").Scan(&count); err != nil || count != 0 {
		t.Fatalf("事务没有回滚商品：%d %v", count, err)
	}
	if _, err := Run(t.Context(), database, true); err == nil {
		t.Fatal("不完整基线应被拒绝")
	}
	// 已取消的 context 也应阻止新批次开始，而不是继续写入后台。
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := insertBatch(ctx, database, 4, 4); err == nil {
		t.Fatal("取消后仍然写入")
	}
}
