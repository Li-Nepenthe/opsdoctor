// 本文件：用真实索引账号验证最后一道数据库权限边界。
//
// 先建立和撤销一个专用测试索引，证明账号确实可用。然后执行不会改动业务行的
// 语句，逐一检查权限错误号；连接失败或语法错误都不能冒充“写入已被拒绝”。
//
// 主要内容：TestIntegrationIndexPermissions 验证 INDEX 能力与越权拒绝。
// 阅读建议：这个测试只认显式的 TEST_SHOP_MYSQL_DDL_DSN，不借用医生或业务账号。

package approval

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// TestIntegrationIndexPermissions 账号误获写权限时也不修改业务行，但测试会立即失败。
func TestIntegrationIndexPermissions(t *testing.T) {
	dsn := os.Getenv("TEST_SHOP_MYSQL_DDL_DSN")
	if dsn == "" {
		t.Skip("设置 TEST_SHOP_MYSQL_DDL_DSN 后验证索引账号权限")
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil || config.User != "diag_ddl" || config.DBName != "shop" {
		t.Fatal("必须使用 shop 的 diag_ddl 账号")
	}
	config.MultiStatements = false
	connector, err := mysql.NewConnector(config)
	if err != nil {
		t.Fatal("测试连接器配置失败")
	}
	database := sql.OpenDB(connector)
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(0)
	defer database.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err = database.PingContext(ctx); err != nil {
		t.Fatal("索引账号无法连接，请核对授权和环境变量")
	}
	// SHOW GRANTS 可以查看自己的授权。先核对只有 USAGE 和五张表的 INDEX，
	// 就不需要真的尝试 DROP TABLE 等危险操作来证明它们不可用。
	grants, err := database.QueryContext(ctx, "SHOW GRANTS")
	if err != nil {
		t.Fatal("无法核对索引账号授权")
	}
	seen := map[string]bool{}
	for grants.Next() {
		var grant string
		if err = grants.Scan(&grant); err != nil {
			grants.Close()
			t.Fatal(err)
		}
		allowed := strings.HasPrefix(grant, "GRANT USAGE ON *.* TO ")
		for _, table := range []string{"products", "stock", "orders", "product_sales", "processed_events"} {
			if strings.HasPrefix(grant, "GRANT INDEX ON `shop`.`"+table+"` TO ") {
				allowed = true
				seen[table] = true
			}
		}
		if !allowed || strings.Contains(grant, "WITH GRANT OPTION") {
			grants.Close()
			t.Fatal("索引账号存在超出约定的授权")
		}
	}
	if err = grants.Err(); err != nil {
		grants.Close()
		t.Fatal(err)
	}
	grants.Close()
	if len(seen) != 5 {
		t.Fatal("索引账号未覆盖架构中的五张表")
	}
	// 用已有主键建一个临时辅助索引，不改变数据；不使用 idx_doctor_ 前缀以免混淆真实审批。
	const index = "idx_permission_test_phase07"
	if _, err = database.ExecContext(ctx, "CREATE INDEX "+index+" ON products (id) ALGORITHM=INPLACE LOCK=NONE"); err != nil {
		t.Fatalf("INDEX 能力未通过：%s", databaseProblem(err))
	}
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()
		// Cleanup 比 defer database.Close 更晚执行，所以这里独立打开同一账号的短连接。
		db := sql.OpenDB(connector)
		defer db.Close()
		if _, err := db.ExecContext(cleanup, "DROP INDEX "+index+" ON products ALGORITHM=INPLACE LOCK=NONE"); err != nil {
			t.Errorf("临时索引未撤销：%s", databaseProblem(err))
		}
	})
	for _, test := range []struct{ name, sql string }{
		{"不能读取业务行", "SELECT * FROM products LIMIT 0"},
		{"不能修改业务行", "UPDATE products SET id=id WHERE 1=0"},
		{"不能删除业务行", "DELETE FROM products WHERE 1=0"},
		{"不能插入业务行", "INSERT INTO products SELECT * FROM products WHERE 1=0"},
		{"不能读取医生库", "SELECT * FROM doctor.diagnoses LIMIT 0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows, err := database.QueryContext(ctx, test.sql)
			if rows != nil {
				_ = rows.Close()
			}
			var denied *mysql.MySQLError
			if !errors.As(err, &denied) || (denied.Number != 1142 && denied.Number != 1044) {
				t.Fatalf("没有明确的权限拒绝：%s", databaseProblem(err))
			}
		})
	}
	t.Log("INDEX 创建成功；SELECT、INSERT、UPDATE、DELETE 与 doctor 库读取均被权限拒绝；临时索引由 Cleanup 撤销")
}
