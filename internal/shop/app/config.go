// 本文件：读取各 shop 程序共用的数据库配置。
//
// 库存服务和 shopctl 都调用 DatabaseConfig，集中维护默认池参数，避免两个入口各写一份后逐渐不一致。
// 先看 open 与 idle 的范围关系，再看多个配置错误如何一起返回。

package app

import (
	"errors"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
)

// DatabaseConfig 统一读取 shop 服务与 shopctl 的数据库配置，避免两套默认连接池参数逐渐不一致。
// 只做本地校验，不联网；errors.Join 会把多个配置问题一起反馈，但不会回显任何配置值。
func DatabaseConfig(reader config.Reader) (mysqlx.Config, error) {
	dsn, dsnErr := reader.String("MYSQL_DSN", "")
	// 本机 MySQL 最多 100 连接，单服务默认只取 20；允许调大到 100 供后续连接池实验使用。
	maxOpen, openErr := reader.Int("DB_MAX_OPEN_CONNS", 20, 1, 100)
	maxIdle, idleErr := reader.Int("DB_MAX_IDLE_CONNS", 10, 0, maxOpen)
	lifetime, lifetimeErr := reader.Duration("DB_CONN_MAX_LIFETIME", 5*time.Minute, time.Second, time.Hour)
	return mysqlx.Config{DSN: dsn, MaxOpenConns: maxOpen, MaxIdleConns: maxIdle, ConnMaxLifetime: lifetime}, errors.Join(dsnErr, openErr, idleErr, lifetimeErr)
}
