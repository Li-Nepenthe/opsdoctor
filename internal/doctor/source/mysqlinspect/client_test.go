// 本文件：用 database/sql 的假驱动检查查询模板和连接排队预算。
//
// MySQL 使用自己的协议，不能用 HTTP 服务器代替。假驱动位于 database/sql 与
// 网络驱动之间，既能看到真正发出的 SQL，也能保留标准库连接池的排队行为。
//
// 主要内容：
//   - TestQueries：检查只查元数据、参数绑定、NULL 和条数限制。
//   - TestPoolDeadline：占住三个连接，用虚拟时钟验证排队也最多十秒。
//   - TestOptions：证明代码覆盖了危险的连接串选项。
//   - fixtureDriver、fixtureConn、fixtureRows：只服务本文件的最小数据库驱动。
//
// 阅读建议：先看 TestQueries 的断言，再读假驱动如何返回两行固定结果。

package mysqlinspect

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"github.com/go-sql-driver/mysql"
)

// fixtureDriver 记录 SQL 与参数，用锁保护数据库连接池可能发起的并发调用。
type fixtureDriver struct {
	mu      sync.Mutex            // 保护下面的记录。
	queries []string              // 原始 SQL 模板，不含运行环境凭据。
	args    [][]driver.NamedValue // 单独保存绑定参数，检查表名没有被拼接。
}

// TestExplainErrors 只公开调用者可自行修正的三类错误，并检查中文截断没有破坏编码。
func TestExplainErrors(t *testing.T) {
	for _, number := range []uint16{1054, 1146, 1064, 1045, 1142} {
		// 前三类分别是列名、表名、语法；后两类可能包含账号，不应返回原文。
		problem := explainError(t.Context(), &mysql.MySQLError{Number: number, Message: "caller_identifier"})
		visible := number == 1054 || number == 1146 || number == 1064
		if strings.Contains(problem.Error(), "caller_identifier") != visible || !strings.Contains(problem.Error(), fmt.Sprint(number)) {
			t.Fatalf("number=%d err=%v", number, problem)
		}
	}
	problem := explainError(t.Context(), &mysql.MySQLError{Number: 1064, Message: strings.Repeat("列", 500)})
	if len(problem.Error()) > 600 || !strings.Contains(problem.Error(), "已截断") || strings.ContainsRune(problem.Error(), '\ufffd') {
		t.Fatal("长错误没有按 UTF-8 边界截断")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if explainError(ctx, &mysql.MySQLError{Number: 1054}) != context.Canceled {
		t.Fatal("取消被错误原文覆盖")
	}
}

// Connect 不建网络连接，让测试只验证 database/sql 这一层。
func (fixture *fixtureDriver) Connect(context.Context) (driver.Conn, error) {
	return &fixtureConn{fixture: fixture}, nil
}

// Driver 返回自身，因为同一个测试类型同时实现了 Connector 和 Driver。
func (fixture *fixtureDriver) Driver() driver.Driver { return fixture }

// Open 是旧式接口的必需方法；测试始终使用 OpenDB，不按字符串注册全局驱动。
func (fixture *fixtureDriver) Open(string) (driver.Conn, error) {
	return fixture.Connect(context.Background())
}

// fixtureConn 不支持事务和写操作，误用时立即报错。
type fixtureConn struct{ fixture *fixtureDriver } // 每条假连接归属于同一个测试记录器。

// Prepare 不属于本测试的元数据读取路径，出现它说明实现改变了。
func (*fixtureConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("unexpected Prepare")
}

// Begin 不提供空事务来掩盖意外的业务操作。
func (*fixtureConn) Begin() (driver.Tx, error) { return nil, fmt.Errorf("unexpected Begin") }

// Close 没有网络资源需要回收，返回成功让标准库正常结束连接生命周期。
func (*fixtureConn) Close() error { return nil }

// QueryContext 记录真实模板并返回可预测的元数据，字段里故意包含 SQL NULL。
func (connection *fixtureConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	fixture := connection.fixture
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.queries = append(fixture.queries, query)
	fixture.args = append(fixture.args, args)
	return &fixtureRows{}, nil
}

// fixtureRows 的两行可以同时检查扫描和超限处理。
type fixtureRows struct{ index int } // 下一行位置，Next 只由 sql.Rows 顺序调用。

// Columns 只给出元数据字段，没有 QUERY_SAMPLE_TEXT。
func (*fixtureRows) Columns() []string { return []string{"DIGEST_TEXT", "possible_keys"} }

// Close 没有底层游标，仍实现接口以模拟 Rows 的正常收尾。
func (*fixtureRows) Close() error { return nil }

// Next 用 EOF 表达读完，不能用一行空值来伪造结束。
func (rows *fixtureRows) Next(values []driver.Value) error {
	if rows.index >= 2 {
		return io.EOF
	}
	rows.index++
	values[0] = "SELECT stock FROM stock WHERE sku_id = ?"
	values[1] = nil
	return nil
}

// fakeClient 使用真实连接池，只替换网络驱动。
func fakeClient(t *testing.T) (*Client, *fixtureDriver) {
	t.Helper()
	fixture := &fixtureDriver{}
	database := sql.OpenDB(fixture)
	database.SetMaxOpenConns(3)
	t.Cleanup(func() { _ = database.Close() })
	return &Client{database: database, schema: "shop"}, fixture
}

// TestQueries 核验每一种查看入口，同时证明表名是参数，EXPLAIN 是工具自己加的前缀。
func TestQueries(t *testing.T) {
	client, fixture := fakeClient(t)
	for _, testcase := range []struct {
		name     string
		call     func() (source.Table, error)
		contains string
	}{
		{"计划", func() (source.Table, error) { return client.Explain(t.Context(), "SELECT * FROM stock WHERE sku_id=1") }, "EXPLAIN SELECT"}, // 不能直接执行输入 SELECT。
		{"索引", func() (source.Table, error) { return client.Indexes(t.Context(), "stock") }, "information_schema.STATISTICS"},               // 表名用问号绑定。
		{"表统计", func() (source.Table, error) { return client.TableStats(t.Context(), "stock") }, "information_schema.TABLES"},               // 不能用 COUNT 扫描业务表。
		{"摘要", func() (source.Table, error) { return client.TopQueries(t.Context(), 1) }, "DIGEST_TEXT"},                                    // 不取真实 SQL 样例。
		{"锁等待", func() (source.Table, error) { return client.LockWaits(t.Context()) }, "performance_schema.data_lock_waits"},                // 不读取锁住的业务行内容。
		{"状态", func() (source.Table, error) { return client.Status(t.Context()) }, "performance_schema.global_status"},                      // 固定状态字段。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			result, err := testcase.call()
			if err != nil {
				t.Fatal(err)
			}
			last := fixture.queries[len(fixture.queries)-1]
			if !strings.Contains(last, testcase.contains) || strings.Contains(last, "QUERY_SAMPLE_TEXT") || strings.Contains(last, "LOCK_DATA") {
				t.Fatal(last)
			}
			if result.Rows[0][1] != "NULL" {
				t.Fatal("NULL 丢失")
			}
			if testcase.name == "摘要" && (!result.Limited || len(result.Rows) != 1) {
				t.Fatal("没有截断多余行")
			}
			if testcase.name == "索引" && (len(fixture.args[len(fixture.args)-1]) != 2 || strings.Contains(last, "'stock'")) {
				t.Fatal("表名没有绑定")
			}
		})
	}
	before := len(fixture.queries)
	if _, err := client.Explain(t.Context(), "UPDATE stock SET stock=0"); err == nil {
		t.Fatal("放行写语句")
	}
	if _, err := client.Indexes(t.Context(), "stock;DELETE"); err == nil {
		t.Fatal("放行非法表名")
	}
	if len(fixture.queries) != before {
		t.Fatal("参数被拒绝前已查询数据库")
	}
}

// TestPoolDeadline 用虚拟时间等待真实连接池，不依赖机器在几毫秒内被调度。
func TestPoolDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client, _ := fakeClient(t)
		for range 3 {
			connection, err := client.database.Conn(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close() // 占满池，第四个调用只能排队。
		}
		started := time.Now()
		_, err := client.Status(t.Context())
		if err != context.DeadlineExceeded || time.Since(started) != 10*time.Second {
			t.Fatalf("err=%v elapsed=%v", err, time.Since(started))
		}
		stats := client.database.Stats()
		if stats.OpenConnections != 3 || stats.WaitCount != 1 {
			t.Fatalf("pool=%+v", stats)
		}
	})
}

// TestOptions 即使部署者误开了多语句或本地文件读取，医生也会在构造时覆盖它。
func TestOptions(t *testing.T) {
	options, err := prepare("diag_ro:demo-only@tcp(localhost:3306)/shop?multiStatements=true&allowAllFiles=true&interpolateParams=true")
	if err != nil {
		t.Fatal(err)
	}
	if options.MultiStatements || options.AllowAllFiles || options.InterpolateParams {
		t.Fatal("危险选项没有关闭")
	}
	for _, dsn := range []string{"", "root:demo@tcp(localhost:3306)/shop", "app:demo@tcp(localhost:3306)/shop", "diag_ro@tcp(localhost:3306)/shop"} {
		// 匿名或业务高权限账号都不符合独立只读身份的要求。
		if _, err := New(dsn); err == nil {
			t.Fatal("接受了无效身份")
		}
	}
}
