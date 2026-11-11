// 本文件：各个 shop 程序从这里读取数据库连接池配置，订单服务和消费者也共用同一套 Kafka 地址校验。

package app

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
)

// DatabaseConfig 统一读取 shop 服务与 shopctl 的数据库配置，避免两套默认连接池参数逐渐不一致。
// 启动时一次报告全部配置问题，避免在错误日志中回显密码。
func DatabaseConfig(reader config.Reader) (mysqlx.Config, error) {
	dsn, dsnErr := reader.String("MYSQL_DSN", "")
	// 本机 MySQL 最多 100 连接，单服务默认只取 20。允许调大到 100 供后续连接池实验使用。
	maxOpen, openErr := reader.Int("DB_MAX_OPEN_CONNS", 20, 1, 100)
	maxIdle, idleErr := reader.Int("DB_MAX_IDLE_CONNS", 10, 0, maxOpen)
	lifetime, lifetimeErr := reader.Duration("DB_CONN_MAX_LIFETIME", 5*time.Minute, time.Second, time.Hour)
	return mysqlx.Config{DSN: dsn, MaxOpenConns: maxOpen, MaxIdleConns: maxIdle, ConnMaxLifetime: lifetime}, errors.Join(dsnErr, openErr, idleErr, lifetimeErr)
}

// KafkaBrokers 要求每项都包含主机和端口，空列表项通常是配置多写了一个逗号。
func KafkaBrokers(raw string) ([]string, error) {
	brokers := strings.Split(raw, ",")
	for index, address := range brokers {
		address = strings.TrimSpace(address)
		if host, port, err := net.SplitHostPort(address); err != nil || host == "" || port == "" {
			return nil, fmt.Errorf("KAFKA_BROKERS 必须是逗号分隔的主机:端口")
		}
		brokers[index] = address
	}
	return brokers, nil
}
