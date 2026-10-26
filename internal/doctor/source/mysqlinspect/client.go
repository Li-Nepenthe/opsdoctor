// 本文件：用一个最多三条连接的只读账号检查 MySQL。
//
// 每条查询都有明确用途，没有把用户文本交给任意 Query 的入口。只有 Explain
// 接受经过检查的 SELECT，并且由这里加 EXPLAIN，永远不执行原 SELECT。
//
// 主要内容：
//   - New、Close：建立惰性连接池，关闭多语句执行并限制连接数。
//   - Explain、Indexes、TableStats：看查询计划和表结构。
//   - TopQueries、LockWaits、Status：看 SQL 摘要、锁等待和运行状态。
//   - query：统一读取有上限的元数据表格。
//
// 阅读建议：先读 New 的限制，再逐一看固定 SQL 取了哪些字段。

package mysqlinspect

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"github.com/go-sql-driver/mysql"
)

// Client 不暴露底层 sql.DB，调用者只能使用下面六种查看方法。
type Client struct {
	database *sql.DB // database/sql 自己管理排队，最多三条连接。
	schema   string  // 固定为配置 DSN 的库名，工具参数不能切换。
}

// New 不立即联网。空库首次启动时，业务表迁移完后才能补授表级权限。
func New(dsn string) (*Client, error) {
	config, err := prepare(dsn)
	if err != nil {
		return nil, err
	}
	connector, err := mysql.NewConnector(config)
	if err != nil {
		return nil, fmt.Errorf("无法创建只读 MySQL 连接器")
	}
	database := sql.OpenDB(connector)
	database.SetMaxOpenConns(3)
	database.SetMaxIdleConns(1)
	database.SetConnMaxLifetime(5 * time.Minute)
	database.SetConnMaxIdleTime(time.Minute)
	return &Client{database: database, schema: config.DBName}, nil
}

// prepare 把连接串中的选项收窄到查看工具需要的范围，防止部署误配打开多语句执行。
func prepare(dsn string) (*mysql.Config, error) {
	config, err := mysql.ParseDSN(dsn)
	if err != nil || config.DBName == "" || config.User == "" || config.Passwd == "" || config.User == "root" || config.User == "app" {
		return nil, fmt.Errorf("只读 MySQL DSN 必须提供独立账号和库名")
	}
	// 即使环境变量误写 multiStatements=true，也由代码强制关闭。
	config.MultiStatements = false
	config.AllowAllFiles = false
	config.InterpolateParams = false
	config.ParseTime = true
	config.Loc = time.UTC
	config.Timeout = 5 * time.Second
	config.ReadTimeout = 10 * time.Second
	config.WriteTimeout = 5 * time.Second
	if config.Params == nil {
		config.Params = make(map[string]string)
	}
	config.Params["time_zone"] = "'+00:00'"
	config.Params["max_execution_time"] = "8000"
	config.Params["lock_wait_timeout"] = "5"
	return config, nil
}

// Close 由整体装配负责调用，工具执行完只归还连接，不关闭共享池。
func (client *Client) Close() error { return client.database.Close() }

// Explain 永远自己加前缀，调用者不能选择 EXPLAIN ANALYZE 去实际执行查询。
func (client *Client) Explain(ctx context.Context, statement string) (source.Table, error) {
	selectSQL, err := source.SelectStatement(statement)
	if err != nil {
		return source.Table{}, err
	}
	return client.query(ctx, "EXPLAIN "+selectSQL, 100)
}

// Indexes 的表名是绑定参数，不拼成 SQL 标识符，名字里有反引号也无法改变语句。
func (client *Client) Indexes(ctx context.Context, table string) (source.Table, error) {
	if !tableName(table) {
		return source.Table{}, fmt.Errorf("表名格式不合法")
	}
	return client.query(ctx, `SELECT INDEX_NAME, NON_UNIQUE, SEQ_IN_INDEX, COLUMN_NAME, CARDINALITY, INDEX_TYPE
FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=? AND TABLE_NAME=? ORDER BY INDEX_NAME,SEQ_IN_INDEX`, 100, client.schema, table)
}

// TableStats 读取的是估计行数和空间用量，不对业务表执行 COUNT(*) 扫描。
func (client *Client) TableStats(ctx context.Context, table string) (source.Table, error) {
	if !tableName(table) {
		return source.Table{}, fmt.Errorf("表名格式不合法")
	}
	return client.query(ctx, `SELECT TABLE_NAME,TABLE_ROWS,DATA_LENGTH,INDEX_LENGTH
FROM information_schema.TABLES WHERE TABLE_SCHEMA=? AND TABLE_NAME=?`, 1, client.schema, table)
}

// TopQueries 只读把具体值换成问号的 DIGEST_TEXT，避免泄露样例 SQL 里的业务内容。
func (client *Client) TopQueries(ctx context.Context, limit int) (source.Table, error) {
	if limit < 1 || limit > 20 {
		return source.Table{}, fmt.Errorf("SQL 摘要条数必须为 1–20")
	}
	return client.query(ctx, `SELECT DIGEST_TEXT,COUNT_STAR,ROUND(SUM_TIMER_WAIT/1000000000000,6) AS total_seconds,
ROUND(AVG_TIMER_WAIT/1000000000,3) AS avg_ms,SUM_ROWS_EXAMINED,SUM_ROWS_SENT
FROM performance_schema.events_statements_summary_by_digest
WHERE SCHEMA_NAME=? AND DIGEST_TEXT IS NOT NULL ORDER BY SUM_TIMER_WAIT DESC LIMIT ?`, limit, client.schema, limit)
}

// LockWaits 直接读等待关系，不读取 LOCK_DATA 或正在执行的原始 SQL。
func (client *Client) LockWaits(ctx context.Context) (source.Table, error) {
	// 直接使用 performance_schema，不需要为 sys 的部分视图额外授予 PROCESS 权限。
	return client.query(ctx, `SELECT r.OBJECT_SCHEMA,r.OBJECT_NAME,r.INDEX_NAME,r.LOCK_TYPE,r.LOCK_MODE,
w.REQUESTING_THREAD_ID,w.BLOCKING_THREAD_ID
FROM performance_schema.data_lock_waits AS w
JOIN performance_schema.data_locks AS r ON r.ENGINE=w.ENGINE AND r.ENGINE_LOCK_ID=w.REQUESTING_ENGINE_LOCK_ID
WHERE r.OBJECT_SCHEMA=? LIMIT 101`, 100, client.schema)
}

// Status 只返回诊断需要的连接、线程和行锁计数，不开放任意 SHOW 命令。
func (client *Client) Status(ctx context.Context) (source.Table, error) {
	return client.query(ctx, `SELECT VARIABLE_NAME,VARIABLE_VALUE FROM performance_schema.global_status
WHERE VARIABLE_NAME IN ('Threads_connected','Threads_running','Max_used_connections','Connections','Slow_queries','Innodb_row_lock_current_waits','Innodb_row_lock_time') ORDER BY VARIABLE_NAME`, 20)
}

// query 的 context 同时覆盖等池连接和读结果；最多十秒，不额外给排队续时。
func (client *Client) query(ctx context.Context, statement string, limit int, args ...any) (source.Table, error) {
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rows, err := client.database.QueryContext(bounded, statement, args...)
	if err != nil {
		return source.Table{}, safeError(bounded, err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return source.Table{}, safeError(bounded, err)
	}
	result := source.Table{Columns: columns, Rows: [][]string{}}
	for rows.Next() {
		if len(result.Rows) == limit {
			result.Limited = true
			break
		}
		values := make([]sql.NullString, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return source.Table{}, safeError(bounded, err)
		}
		line := make([]string, len(columns))
		for index, value := range values {
			// NullString 区分数据库 NULL 与空字符串，不能把“缺少索引”误显示成空索引名。
			line[index] = "NULL"
			if value.Valid {
				line[index] = value.String
			}
		}
		result.Rows = append(result.Rows, line)
	}
	if err := rows.Err(); err != nil {
		return source.Table{}, safeError(bounded, err)
	}
	return result, nil
}

// safeError 保留错误号方便排查授权，不回显可能带 SQL 或账号信息的数据库原文。
func safeError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var databaseError *mysql.MySQLError
	if errors.As(err, &databaseError) {
		return fmt.Errorf("MySQL 查询失败，错误号 %d", databaseError.Number)
	}
	return fmt.Errorf("MySQL 查询未完成")
}

// tableName 接受常规表名；真正的库名只来自构造配置。
func tableName(value string) bool {
	return regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`).MatchString(value)
}
