// 本文件：库存进程的资源组装与退出流程。
//
// cmd 入口把控制权交给 Run，它连接数据库、迁移并串起仓储、业务和 gRPC。
// 先读 Run 的资源所有权，再读 serve 的两个监听器与结果 channel。只有入口关闭后才归还数据库资源。
//
// 主要内容：
//   - Run：拥有库存进程的完整生命周期。具名 result 允许 defer 把关闭连接池的错误也返回给 main。
//   - serve：在两个端口都绑定成功后才接收请求。任意 Serve 异常都触发整组停机。
//   - normalServeError：只过滤预期的关闭结果。端口故障等错误仍返回给进程入口。

package inventoryservice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"

	inventoryv1 "github.com/Li-Nepenthe/opsdoctor/gen/inventory/v1"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/adminserver"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/grpcx"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/lifecycle"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/logging"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/inventory"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/migrations"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

// Run 拥有库存进程的完整生命周期。具名 result 允许 defer 把关闭连接池的错误也返回给 main。
func Run(parent context.Context) (result error) {
	settings, err := loadConfig(config.New())
	if err != nil {
		return err
	}
	observability, err := telemetry.New(parent, "inventory-service")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, observability.Shutdown(parent)) }()
	settings.Database.TracerProvider = observability.Provider
	logger := logging.New(os.Stdout, observability.Service, slog.LevelInfo)
	ctx, stop := lifecycle.Notify(parent)
	defer stop()
	// 启动使用独立的短预算。成功后取消它释放定时器，不能把它传给长期运行的服务器。
	startup, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	database, err := mysqlx.Open(startup, settings.Database)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, database.Close()) }()
	if err := observability.Metrics.Database(database, "shop"); err != nil {
		return err
	}
	if err := migrations.Apply(startup, database); err != nil {
		return fmt.Errorf("迁移 shop：%w", err)
	}
	cancel()
	server := grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(observability.Provider), otelgrpc.WithPropagators(observability.Propagator))), grpc.ChainUnaryInterceptor(observability.Metrics.Server(), grpcx.Logging(logger), grpcx.Recovery(logger)))
	service := inventory.NewService(inventory.NewMySQLStockRepository(database))
	inventoryv1.RegisterInventoryServiceServer(server, inventory.NewGRPCServer(service))
	admin := adminserver.New(settings.AdminAddress, database.PingContext, observability.Metrics.Registry)
	// 只输出无敏感信息的生效参数。日志中不出现 DSN 或整个配置结构体。
	logger.Info("库存服务配置已加载", "version", telemetry.Version, "grpc_addr", settings.GRPCAddress, "admin_addr", settings.AdminAddress,
		"db_max_open_conns", settings.Database.MaxOpenConns, "db_max_idle_conns", settings.Database.MaxIdleConns,
		"db_conn_max_lifetime", settings.Database.ConnMaxLifetime.String())
	return serve(ctx, settings.GRPCAddress, server, admin, logger)
}

// serve 在两个端口都绑定成功后才接收请求。任意 Serve 异常都触发整组停机。
func serve(ctx context.Context, address string, server *grpc.Server, admin *http.Server, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("监听库存 gRPC：%w", err)
	}
	adminListener, err := net.Listen("tcp", admin.Addr)
	if err != nil {
		return errors.Join(fmt.Errorf("监听管理端口：%w", err), listener.Close())
	}
	// 两个 Serve 各写一次结果，主流程最多暂时不读两个结果，所以容量是 2。无需关闭 channel。
	finished := make(chan error, 2)
	go func() { finished <- server.Serve(listener) }()
	go func() { finished <- admin.Serve(adminListener) }()
	// Serve 的 goroutine 由下面的 Shutdown/Stop 退出。返回前读齐结果，证明没有遗留监听任务。
	logger.Info("库存服务已启动")
	var serveErr error
	remaining := 2
	select {
	case <-ctx.Done():
		logger.Info("收到退出信号，开始排空库存请求")
	case serveErr = <-finished:
		remaining--
	}
	serveErr = normalServeError(serveErr)
	closeErr := lifecycle.Shutdown(ctx, shutdownTimeout,
		lifecycle.Hook{Name: "gRPC", Close: func(cleanup context.Context) error { return grpcx.Shutdown(cleanup, server) }},
		lifecycle.Hook{Name: "管理端口", Close: func(cleanup context.Context) error { return adminserver.Shutdown(cleanup, admin) }})
	for range remaining {
		// 正常关闭会得到标准关闭哨兵。其他 Serve 错误必须保留，不能静默退出为成功。
		serveErr = errors.Join(serveErr, normalServeError(<-finished))
	}
	logger.Info("库存入口已关闭，准备关闭数据库连接池")
	return errors.Join(serveErr, closeErr)
}

// normalServeError 只过滤预期的关闭结果。端口故障等错误仍返回给进程入口。
func normalServeError(err error) error {
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, grpc.ErrServerStopped) {
		return nil
	}
	return err
}
