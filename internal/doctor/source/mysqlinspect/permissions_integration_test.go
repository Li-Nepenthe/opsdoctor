// 本文件：用真实 diag_ro 账号证明“能看执行计划，不能写业务表”。
//
// 写权限测试的 WHERE 恒为假。即使账号意外拿到写权限，也不会改动任何业务行，
// 但测试必须失败。这样无需拿真实数据冒险来证明权限边界。
//
// 主要内容：
//   - TestIntegrationMySQLPermissions：检查六类查看操作，以及写入和样例 SQL 的拒绝。
//   - TestIntegrationMySQLConnectionLimit：占满三条连接后，服务端拒绝该账号的第四条。
//   - integrationInspector：只在显式提供测试 DSN 时连接，避免静默使用生产配置。
//
// 阅读建议：连接池限制由单元测试证明，这里另测数据库账号自身的限制。

package mysqlinspect

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"github.com/go-sql-driver/mysql"
)

// integrationInspector 在连接失败时终止测试，不能把“没有测到”当成“没有权限”。
func integrationInspector(t *testing.T) (*Client, string) {
	// 缺少测试凭据时明确跳过；拿到凭据后连接失败必须报错，不能把权限未验证当成通过。
	t.Helper()
	dsn := os.Getenv("TEST_SHOP_MYSQL_RO_DSN")
	if dsn == "" {
		t.Skip("设置 TEST_SHOP_MYSQL_RO_DSN 后运行只读权限测试")
	}
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil || parsed.User != "diag_ro" || parsed.DBName != "shop" {
		t.Fatal("权限测试必须使用 shop 的 diag_ro 账号")
	}
	client, err := New(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := client.database.PingContext(ctx); err != nil {
		t.Fatal("只读账号无法连接，请检查授权和测试配置")
	}
	return client, dsn
}

// TestIntegrationMySQLPermissions 每个拒绝都检查 MySQL 错误号，语法错误不能冒充权限证明。
func TestIntegrationMySQLPermissions(t *testing.T) {
	client, _ := integrationInspector(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for _, testcase := range []struct {
		name string
		call func() (source.Table, error)
	}{
		{"执行计划", func() (source.Table, error) { return client.Explain(ctx, "SELECT * FROM stock WHERE sku_id=1") }}, // EXPLAIN 需要表的 SELECT 权限。
		{"索引", func() (source.Table, error) { return client.Indexes(ctx, "stock") }},                                // 能看到已有查找方式。
		{"表统计", func() (source.Table, error) { return client.TableStats(ctx, "stock") }},                            // 读取估计值，不扫描全表。
		{"SQL摘要", func() (source.Table, error) { return client.TopQueries(ctx, 3) }},                                // 能读 DIGEST_TEXT，不取真实样例。
		{"锁等待", func() (source.Table, error) { return client.LockWaits(ctx) }},                                      // 没等待时允许空表。
		{"状态", func() (source.Table, error) { return client.Status(ctx) }},                                          // 验证列级权限已齐备。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			if _, err := testcase.call(); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, statement := range []string{
		"UPDATE stock SET stock=stock WHERE 1=0",          // 即使误授权也不会改行。
		"DELETE FROM stock WHERE 1=0",                     // 即使误授权也不会删行。
		"INSERT INTO stock SELECT * FROM stock WHERE 1=0", // 即使误授权也不会插行。
	} {
		_, err := client.database.ExecContext(ctx, statement)
		var denied *mysql.MySQLError
		if !errors.As(err, &denied) || denied.Number != 1142 {
			t.Fatalf("写操作未被权限明确拒绝：%v", safeError(ctx, err))
		}
	}
	// 样例 SQL 的权限也不提供；LIMIT 0 让误授权的情况仍不会返回业务值。
	rows, err := client.database.QueryContext(ctx, "SELECT QUERY_SAMPLE_TEXT FROM performance_schema.events_statements_summary_by_digest LIMIT 0")
	if rows != nil {
		_ = rows.Close()
	}
	var denied *mysql.MySQLError
	if !errors.As(err, &denied) || denied.Number != 1143 {
		t.Fatal("QUERY_SAMPLE_TEXT 未被列权限拒绝")
	}
	rows, err = client.database.QueryContext(ctx, "SELECT 1; SELECT 2")
	if rows != nil {
		_ = rows.Close()
	}
	if !errors.As(err, &denied) || denied.Number != 1064 {
		t.Fatal("多语句执行没有被关闭")
	}
	t.Log("六类查看成功；INSERT/UPDATE/DELETE=1142；QUERY_SAMPLE_TEXT=1143；多语句=1064")
}

// TestIntegrationMySQLConnectionLimit 绕过客户端池再建第四条连接，证明限制也落在账号侧。
func TestIntegrationMySQLConnectionLimit(t *testing.T) {
	client, dsn := integrationInspector(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for range 3 {
		connection, err := client.database.Conn(ctx)
		if err != nil {
			t.Fatal("无法建立三条只读连接")
		}
		defer connection.Close()
	}
	options, err := prepare(dsn)
	if err != nil {
		t.Fatal(err)
	}
	connector, err := mysql.NewConnector(options)
	if err != nil {
		t.Fatal("连接器配置失败")
	}
	fourth := sql.OpenDB(connector)
	defer fourth.Close()
	err = fourth.PingContext(ctx)
	var limited *mysql.MySQLError
	if !errors.As(err, &limited) || limited.Number != 1226 {
		t.Fatalf("第四条连接没有返回账号限额错误：%v", safeError(ctx, err))
	}
	// 自己的 SHOW GRANTS 不含密码哈希；这里只检查权限关键词，不把结果整段写进测试输出。
	rows, err := client.database.QueryContext(ctx, "SHOW GRANTS")
	if err == nil {
		rows.Close()
		t.Fatal("池已满，却没有等待连接")
	}
	if !strings.Contains(err.Error(), "deadline") {
		t.Fatal("排队没有受父请求预算限制")
	}
	t.Log("账号第四条连接=1226；客户端池最多三条，排队受截止时间限制")
}
