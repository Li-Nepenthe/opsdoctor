// 本文件：在消费者连接数据库和 Kafka 前，读取管理端口、消息时限和依赖地址。

package orderworker

import (
	"errors"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/app"
)

// Config 保存消费者的启动参数。数据库连接串可能含有密码，日志只应记录端口和消息时限。
type Config struct {
	Database       mysqlx.Config
	AdminAddress   string
	KafkaBrokers   []string
	MessageTimeout time.Duration
}

// loadConfig 先一起返回环境变量读取错误；全部读到后才检查每个 Kafka 地址的格式。
func loadConfig(reader config.Reader) (Config, error) {
	database, databaseErr := app.DatabaseConfig(reader)
	address, addressErr := reader.String("ADMIN_ADDR", ":6063")
	rawBrokers, brokersErr := reader.String("KAFKA_BROKERS", "127.0.0.1:9092")
	messageTimeout, timeoutErr := reader.Duration("WORKER_MESSAGE_TIMEOUT", 10*time.Second, time.Second, 30*time.Second)
	if err := errors.Join(databaseErr, addressErr, brokersErr, timeoutErr); err != nil {
		return Config{}, err
	}
	brokers, err := app.KafkaBrokers(rawBrokers)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Database:       database,
		AdminAddress:   address,
		KafkaBrokers:   brokers,
		MessageTimeout: messageTimeout,
	}, nil
}
