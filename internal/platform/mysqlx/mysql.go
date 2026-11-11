// 本文件：服务启动时调用 Open，建立 MySQL 连接池并确认数据库可达。
// prepare 在连接前校验配置，所有使用这个池的仓储都会继承同一套时区和超时设置。

package mysqlx

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/go-sql-driver/mysql"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// 网络超时给驱动设上界。业务查询仍应使用自己的截止时间。
const (
	connectTimeout = 5 * time.Second
	readTimeout    = 30 * time.Second // 为迁移等较长 SQL 留出时间。
	writeTimeout   = 30 * time.Second
	idleTimeout    = time.Minute // 及时归还空闲会话，给本机其他服务留下连接。
)

// Config 指定单个服务的连接预算和追踪配置。
type Config struct {
	DSN             string // 含数据库密码，禁止写入日志或错误。
	MaxOpenConns    int    // 每个服务都必须明确设置连接上限。
	MaxIdleConns    int    // 允许为零，上限是 MaxOpenConns。
	ConnMaxLifetime time.Duration
	TracerProvider  trace.TracerProvider // 命令行可留空，沿用普通连接。
}

// Open 返回已通过连通性检查的池，调用方负责在停机时关闭它。
func Open(ctx context.Context, config Config) (*sql.DB, error) {
	driverConfig, err := prepare(config)
	if err != nil {
		return nil, err
	}
	connector, err := mysql.NewConnector(driverConfig)
	if err != nil {
		return nil, fmt.Errorf("创建 MySQL 连接器：配置不合法")
	}
	var database *sql.DB
	if config.TracerProvider == nil {
		database = sql.OpenDB(connector)
	} else {
		// 调用链只记录 SQL 模板，避免参数里的业务数据进入追踪系统。
		database = otelsql.OpenDB(connector, otelsql.WithTracerProvider(config.TracerProvider),
			otelsql.WithAttributes(attribute.String("db.system.name", "mysql"), attribute.String("db.namespace", driverConfig.DBName)),
			otelsql.WithSpanOptions(otelsql.SpanOptions{OmitConnResetSession: true, OmitRows: true, OmitConnectorConnect: true, DisableErrSkip: true}))
	}
	database.SetMaxOpenConns(config.MaxOpenConns)
	database.SetMaxIdleConns(config.MaxIdleConns)
	database.SetConnMaxLifetime(config.ConnMaxLifetime)
	database.SetConnMaxIdleTime(idleTimeout)
	// 启动时主动探测，让无法连接数据库的服务在接收请求前退出。
	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err := database.PingContext(pingCtx); err != nil {
		closeErr := database.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("探测 MySQL：%w；关闭失败的连接池：%v", err, closeErr)
		}
		return nil, fmt.Errorf("探测 MySQL：%w", err)
	}
	return database, nil
}

func prepare(config Config) (*mysql.Config, error) {
	if config.MaxOpenConns <= 0 {
		// 连接数设为零会取消上限，一个服务就可能占满整台 MySQL 的连接。
		return nil, fmt.Errorf("DB_MAX_OPEN_CONNS 必须大于 0")
	}
	if config.MaxIdleConns < 0 || config.MaxIdleConns > config.MaxOpenConns {
		return nil, fmt.Errorf("DB_MAX_IDLE_CONNS 必须在 0 到 DB_MAX_OPEN_CONNS 之间")
	}
	if config.ConnMaxLifetime <= 0 {
		return nil, fmt.Errorf("DB_CONN_MAX_LIFETIME 必须大于 0")
	}
	parsed, err := mysql.ParseDSN(config.DSN)
	if err != nil || config.DSN == "" {
		// 解析错误可能引用含密码的 DSN，向调用方只报告配置项。
		return nil, fmt.Errorf("MySQL DSN 格式不合法，请检查对应环境变量")
	}
	// loc 决定 Go 如何解码日期，time_zone 决定 MySQL 会话使用的时区。两端都要设为 UTC。
	parsed.ParseTime = true
	parsed.Loc = time.UTC
	// 驱动禁用多语句，迁移必须一份文件只放一条 SQL。
	parsed.MultiStatements = false
	if parsed.Params == nil {
		parsed.Params = make(map[string]string)
	}
	parsed.Params["time_zone"] = "'+00:00'"
	parsed.Timeout = connectTimeout
	parsed.ReadTimeout = readTimeout
	parsed.WriteTimeout = writeTimeout
	return parsed, nil
}
