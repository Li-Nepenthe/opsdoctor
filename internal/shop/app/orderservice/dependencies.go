// 本文件：准备订单进程需要的连接，并明确由谁关闭它们。
//
// 监控接入后，Run 里挤进了连接、探活、注册和清理细节。把这些步骤搬到这里，
// 入口就可以按“读配置 → 准备连接 → 装配服务 → 接流量”的顺序阅读。
//
// 主要内容：
//   - dependencies、openDependencies：按依赖顺序建立连接，失败时收回已经打开的资源。
//   - openRedis：先装追踪插件，再探测 Redis 是否可用。
//   - openInventory：创建带追踪和指标的连接，并等待首次握手完成。
//   - inventoryDialOptions：集中说明 DNS 查询、空闲回收和监控选项。
//   - close：按与打开相反的顺序归还连接。
//
// 阅读建议：先看 openDependencies 的失败分支，再看 Run 成功以后安排的 defer。

package orderservice

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	inventoryv1 "github.com/Li-Nepenthe/opsdoctor/gen/inventory/v1"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/events"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/inventoryclient"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/migrations"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// dependencies 只保存进程拥有的连接。业务服务借用它们，不负责关闭。
type dependencies struct {
	database   *sql.DB                 // 商品、订单和管理探活共用连接池。
	redis      *redis.Client           // 缓存与幂等占位共用客户端。
	connection *grpc.ClientConn        // 库存客户端复用的传输连接。
	stock      *inventoryclient.Client // 在连接之上提供超时、重试和熔断。
	publisher  *events.Publisher       // 订单提交后发送消息。
}

// openDependencies 失败时自己清理；成功后把关闭责任交给 Run，避免半途失败留下连接。
func openDependencies(ctx context.Context, settings Config, runtime *telemetry.Runtime, logger *slog.Logger) (_ *dependencies, result error) {
	resources := &dependencies{}
	defer func() {
		if result != nil {
			result = errors.Join(result, resources.close())
		}
	}()
	var err error
	resources.database, err = mysqlx.Open(ctx, settings.Database)
	if err != nil {
		return nil, err
	}
	if err := runtime.Metrics.Database(resources.database, "shop"); err != nil {
		return nil, err
	}
	if err := migrations.Apply(ctx, resources.database); err != nil {
		return nil, err
	}
	resources.redis, err = openRedis(ctx, settings.RedisAddress, settings.RedisPassword, runtime)
	if err != nil {
		return nil, err
	}
	resources.connection, err = openInventory(ctx, settings.InventoryTarget, settings.InventoryIdleTimeout, runtime)
	if err != nil {
		return nil, err
	}
	resources.stock, err = inventoryclient.New(inventoryv1.NewInventoryServiceClient(resources.connection), settings.Inventory, logger)
	if err != nil {
		return nil, err
	}
	if err := resources.stock.RegisterMetrics(runtime.Metrics); err != nil {
		return nil, err
	}
	// 自动建主题仍关闭。确认三个分区准备好，才允许接收订单。
	if err := events.EnsureTopic(ctx, settings.KafkaBrokers); err != nil {
		return nil, err
	}
	resources.publisher = events.NewPublisher(settings.KafkaBrokers, runtime, logger)
	return resources, nil
}

// openRedis 不记录命令参数，因为幂等键和缓存正文可能包含请求数据。
func openRedis(ctx context.Context, address, password string, runtime *telemetry.Runtime) (*redis.Client, error) {
	// 禁止自动重试。SET NX 的响应丢失后，重发可能把自己的占位误认为别人的请求。
	client := redis.NewClient(&redis.Options{
		Addr: address, Password: password, MaxRetries: -1, PoolSize: redisPoolSize,
		DialTimeout: redisIOTimeout, ReadTimeout: redisIOTimeout, WriteTimeout: redisIOTimeout,
		ContextTimeoutEnabled: true,
	})
	if err := redisotel.InstrumentTracing(client,
		redisotel.WithTracerProvider(runtime.Provider), redisotel.WithDBStatement(false)); err != nil {
		return nil, errors.Join(err, client.Close())
	}
	if err := client.Ping(ctx).Err(); err != nil {
		// 连接错误可能带地址；启动信息只说明哪个依赖失败。
		return nil, errors.Join(fmt.Errorf("启动时无法访问 Redis"), client.Close())
	}
	return client, nil
}

// openInventory 把插件放到每次真实 RPC 外层，重试发生几次就记录几次调用。
func openInventory(ctx context.Context, target string, idleTimeout time.Duration, runtime *telemetry.Runtime) (*grpc.ClientConn, error) {
	connection, err := grpc.NewClient(target, inventoryDialOptions(idleTimeout, runtime)...)
	if err != nil {
		return nil, fmt.Errorf("创建库存连接：%w", err)
	}
	// NewClient 还没完成网络握手。等待 Ready，第一笔订单才不会承担冷启动连接时间。
	if err := connectInventory(ctx, connection); err != nil {
		return nil, errors.Join(err, connection.Close())
	}
	return connection, nil
}

// inventoryDialOptions 保留按主机名找 IP 的能力，但不从 DNS 读取本项目不用的服务配置。
// 测试也使用同一组选项，确认首次解析和空闲后重建都收到“无需查询服务配置”的标志。
func inventoryDialOptions(idleTimeout time.Duration, runtime *telemetry.Runtime) []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry(),
		// grpc-go 默认在查完 IP 后再查 _grpc_config.<主机名> 的 TXT 记录。
		// Docker 的这次 TXT 查询会超时，超过库存的 800ms 预算；首次建连和空闲重连都会受影响。
		// 服务配置由本项目代码决定，所以跳过 TXT 查询，普通的 IP 解析仍然保留。
		grpc.WithDisableServiceConfig(),
		// 默认仍是 30 分钟。可临时缩短来验证空闲重连，不能靠禁用回收掩盖 DNS 问题。
		grpc.WithIdleTimeout(idleTimeout),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler(
			otelgrpc.WithTracerProvider(runtime.Provider), otelgrpc.WithPropagators(runtime.Propagator))),
		grpc.WithUnaryInterceptor(runtime.Metrics.Client("inventory-service")),
	}
}

// close 既用于启动失败，也用于正常停机，所以只关闭实际打开过的资源。
func (resources *dependencies) close() error {
	var result error
	if resources.publisher != nil {
		result = errors.Join(result, resources.publisher.Close())
	}
	if resources.connection != nil {
		result = errors.Join(result, resources.connection.Close())
	}
	if resources.redis != nil {
		result = errors.Join(result, resources.redis.Close())
	}
	if resources.database != nil {
		result = errors.Join(result, resources.database.Close())
	}
	return result
}
