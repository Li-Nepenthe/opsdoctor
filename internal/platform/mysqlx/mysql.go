package mysqlx

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
)

// 这些超时控制网络和资源回收；它们不代替每次请求自己的 context deadline。
const (
	connectTimeout = 5 * time.Second  // 本机依赖正常时应很快连接，5 秒让启动失败有明确上界。
	readTimeout    = 30 * time.Second // 给迁移等较长 SQL 留余量，业务查询应使用更短 deadline。
	writeTimeout   = 30 * time.Second // 防止网络故障导致发送 SQL 无限等待。
	idleTimeout    = time.Minute      // 尽快回收闲置连接，避免小型靶场常驻太多数据库会话。
)

// Config 描述一个服务独占的连接池，实际环境变量在服务 app 包读取。
type Config struct {
	DSN             string        // DSN 来自 MYSQL_DSN 或 DOCTOR_MYSQL_DSN，包含密码，禁止写日志。
	MaxOpenConns    int           // DB_MAX_OPEN_CONNS，必须大于 0，限制同一服务的并发连接数。
	MaxIdleConns    int           // DB_MAX_IDLE_CONNS，范围为 0 到 MaxOpenConns。
	ConnMaxLifetime time.Duration // DB_CONN_MAX_LIFETIME，必须大于 0，单位为 Duration。
}

// Open 创建连接池并 Ping 验证可达性；最多等待 connectTimeout 或更短的 ctx deadline。
// 成功返回可并发使用的 *sql.DB；失败返回带上下文的错误并关闭新建池，DSN 解析错误不会回显输入。
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
	database := sql.OpenDB(connector)
	database.SetMaxOpenConns(config.MaxOpenConns)
	database.SetMaxIdleConns(config.MaxIdleConns)
	database.SetConnMaxLifetime(config.ConnMaxLifetime)
	database.SetConnMaxIdleTime(idleTimeout)
	// 第 2 步：OpenDB 本身不联网，必须 Ping 才能在启动时发现依赖不可用。
	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err := database.PingContext(pingCtx); err != nil {
		// 失败的池不会交给调用方；Close 的清理错误与主要错误一起保留。
		closeErr := database.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("探测 MySQL：%w；关闭失败的连接池：%v", err, closeErr)
		}
		return nil, fmt.Errorf("探测 MySQL：%w", err)
	}
	return database, nil
}

// prepare 校验连接池参数并规范驱动设置，不执行 IO；解析失败不保留可能包含密码的底层错误。
func prepare(config Config) (*mysql.Config, error) {
	if config.MaxOpenConns <= 0 {
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
		return nil, fmt.Errorf("MySQL DSN 格式不合法，请检查对应环境变量")
	}
	// loc 只影响 Go 解码日期；time_zone 才会让每条新连接的 MySQL 会话使用 UTC。
	parsed.ParseTime = true
	parsed.Loc = time.UTC
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
