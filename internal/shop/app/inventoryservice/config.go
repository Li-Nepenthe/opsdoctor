// 本文件：库存进程自己的监听地址与启动关闭预算。
//
// loadConfig 把共用数据库配置和本服务端口放在一起，供 Run 建立资源。
// 先看 Config 的环境变量对应关系，再看 30 秒启动与 10 秒关闭预算。它们不是每次 RPC 的业务超时。
//
// 主要内容：
//   - loadConfig：汇总配置错误，让用户在建立连接和监听端口前，一次看到所有缺失项。

package inventoryservice

import (
	"errors"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/app"
)

// 启动与停机各有独立预算。它们不覆盖业务 RPC，业务使用调用方自己的 deadline。
const (
	startupTimeout  = 30 * time.Second // 本机迁移只有五张空表，异常时不无限等待启动。
	shutdownTimeout = 10 * time.Second // 小型靶场给在途请求十秒，之后强制取消。
)

// Config 只保存该服务实际使用的变量。不预读阶段 2/3 尚未实现的能力。
type Config struct {
	Database     mysqlx.Config // MYSQL_DSN 与 DB_*；含密码，禁止输出整个 Config。
	GRPCAddress  string        // GRPC_ADDR，默认 :9001；Compose 仅映射到 127.0.0.1。
	AdminAddress string        // ADMIN_ADDR，库存服务默认为 :6062。
}

// loadConfig 汇总配置错误，让用户在建立连接和监听端口前，一次看到所有缺失项。
func loadConfig(reader config.Reader) (Config, error) {
	database, databaseErr := app.DatabaseConfig(reader)
	grpcAddress, grpcErr := reader.String("GRPC_ADDR", ":9001")
	adminAddress, adminErr := reader.String("ADMIN_ADDR", ":6062")
	return Config{Database: database, GRPCAddress: grpcAddress, AdminAddress: adminAddress}, errors.Join(databaseErr, grpcErr, adminErr)
}
