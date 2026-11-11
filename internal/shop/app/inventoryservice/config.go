// 本文件：库存进程在启动连接和监听前，从这里取得两个端口和数据库配置。

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

// Config 保存库存进程的数据库和监听地址；指标、调用链与运维入口由各自的组装函数读取配置。
type Config struct {
	Database     mysqlx.Config
	GRPCAddress  string
	AdminAddress string
}

// loadConfig 汇总配置错误，让用户在建立连接和监听端口前，一次看到所有缺失项。
func loadConfig(reader config.Reader) (Config, error) {
	database, databaseErr := app.DatabaseConfig(reader)
	grpcAddress, grpcErr := reader.String("GRPC_ADDR", ":9001")
	adminAddress, adminErr := reader.String("ADMIN_ADDR", ":6062")
	return Config{Database: database, GRPCAddress: grpcAddress, AdminAddress: adminAddress}, errors.Join(databaseErr, grpcErr, adminErr)
}
