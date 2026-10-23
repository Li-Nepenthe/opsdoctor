// 本文件：建立经过校验的 MySQL 连接池。
//
// app 把配置交给 Open，prepare 统一驱动选项，Ping 再确认数据库可达。
// 把配置和连接动作分开，单元测试才能不用联网。
// 先读连接预算常量和 Config，再看 loc 与会话 time_zone 为什么都要设置。
//
// 主要内容：
//   - Open：创建连接池并 Ping 验证可达性。最多等待 connectTimeout 或更短的 ctx deadline。
//   - prepare：在联网前统一池参数、时区和驱动限制。解析失败只返回配置提示，不能把含密码的 DSN 带进错误。

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

// 这些超时控制网络和资源回收。它们不代替每次请求自己的 context deadline。
const (
	connectTimeout = 5 * time.Second  // 本机依赖正常时应很快连接，5 秒让启动失败有明确上界。
	readTimeout    = 30 * time.Second // 给迁移等较长 SQL 留余量，业务查询应使用更短 deadline。
	writeTimeout   = 30 * time.Second // 防止网络故障导致发送 SQL 无限等待。
	idleTimeout    = time.Minute      // 尽快回收闲置连接，避免小型靶场常驻太多数据库会话。
)

// Config 描述一个服务独占的连接池，实际环境变量在服务 app 包读取。
type Config struct {
	DSN             string               // DSN 来自 MYSQL_DSN 或 DOCTOR_MYSQL_DSN，包含密码，禁止写日志。
	MaxOpenConns    int                  // DB_MAX_OPEN_CONNS，必须大于 0，限制同一服务的并发连接数。
	MaxIdleConns    int                  // DB_MAX_IDLE_CONNS，范围为 0 到 MaxOpenConns。
	ConnMaxLifetime time.Duration        // DB_CONN_MAX_LIFETIME，必须大于 0，单位为 Duration。
	TracerProvider  trace.TracerProvider // app 注入调用链；命令行与原有连接测试可留空。
}

// Open 创建连接池并 Ping 验证可达性。最多等待 connectTimeout 或更短的 ctx deadline。
// *sql.DB 代表连接池，不是一条连接。创建成功后由 app 持有并在停机时关闭，仓储只借它执行查询。
func Open(ctx context.Context, config Config) (*sql.DB, error) {
	// 第 1 步：在连接前校验配置，避免把配置错误误判为网络故障。
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
		// 手动管理命令不需要启动追踪后台任务，保留普通连接方式。
		database = sql.OpenDB(connector)
	} else {
		// 插件记录带 ? 的 SQL 模板，不记录参数值。诊断需要知道慢在哪条 SQL，但不需要业务行内容。
		database = otelsql.OpenDB(connector, otelsql.WithTracerProvider(config.TracerProvider),
			otelsql.WithAttributes(attribute.String("db.system.name", "mysql"), attribute.String("db.namespace", driverConfig.DBName)),
			otelsql.WithSpanOptions(otelsql.SpanOptions{OmitConnResetSession: true, OmitRows: true, OmitConnectorConnect: true, DisableErrSkip: true}))
	}
	database.SetMaxOpenConns(config.MaxOpenConns)
	database.SetMaxIdleConns(config.MaxIdleConns)
	database.SetConnMaxLifetime(config.ConnMaxLifetime)
	database.SetConnMaxIdleTime(idleTimeout)
	// 第 2 步：OpenDB 本身不联网，必须 Ping 才能在启动时发现依赖不可用。
	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err := database.PingContext(pingCtx); err != nil {
		// 失败的池不会交给调用方。Close 的清理错误与主要错误一起保留。
		closeErr := database.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("探测 MySQL：%w；关闭失败的连接池：%v", err, closeErr)
		}
		return nil, fmt.Errorf("探测 MySQL：%w", err)
	}
	return database, nil
}

// prepare 在联网前统一池参数、时区和驱动限制。解析失败只返回配置提示，不能把含密码的 DSN 带进错误。
func prepare(config Config) (*mysql.Config, error) {
	if config.MaxOpenConns <= 0 {
		// database/sql 把 0 理解为不限制连接数。本项目要求显式上限，避免压垮本机 MySQL。
		return nil, fmt.Errorf("DB_MAX_OPEN_CONNS 必须大于 0")
	}
	if config.MaxIdleConns < 0 || config.MaxIdleConns > config.MaxOpenConns {
		// 空闲连接是总连接的一部分，不能比总数多。负数也不是有效的容量设置。
		return nil, fmt.Errorf("DB_MAX_IDLE_CONNS 必须在 0 到 DB_MAX_OPEN_CONNS 之间")
	}
	if config.ConnMaxLifetime <= 0 {
		// 标准库允许连接不过期，但本项目要求定期更换旧连接，因此拒绝无效寿命。
		return nil, fmt.Errorf("DB_CONN_MAX_LIFETIME 必须大于 0")
	}
	parsed, err := mysql.ParseDSN(config.DSN)
	if err != nil || config.DSN == "" {
		// 底层解析错误可能带出 DSN 原文，只返回配置提示，不能把数据库密码写进错误。
		return nil, fmt.Errorf("MySQL DSN 格式不合法，请检查对应环境变量")
	}
	// loc 只影响 Go 解码日期。time_zone 才会让每条新连接的 MySQL 会话使用 UTC。
	parsed.ParseTime = true
	parsed.Loc = time.UTC
	parsed.MultiStatements = false
	// 关闭多语句后，一份迁移不能夹带多条独立 SQL。迁移文件必须遵守“一文件一语句”。
	if parsed.Params == nil {
		// 未提供额外参数时 map 可能是 nil。先分配它才能写入会话时区。
		parsed.Params = make(map[string]string)
	}
	parsed.Params["time_zone"] = "'+00:00'"
	parsed.Timeout = connectTimeout
	parsed.ReadTimeout = readTimeout
	parsed.WriteTimeout = writeTimeout
	return parsed, nil
}
