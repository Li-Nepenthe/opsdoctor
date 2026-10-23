// 本文件：验证订单服务在首次库存连接就绪后才开放入口。
//
// 用 gRPC 的内存连接进行握手，不监听真实网络端口。失败测试直接返回拨号错误。
//
// 主要内容：
//   - TestConnectInventory：检查握手成功和启动预算耗尽两条路径。
//   - TestInventoryIdleReconnect：真实回收并重建内存连接，检查两次解析都禁用服务配置。
//   - memoryInventoryConnection：准备独立解析器、内存服务和清理步骤。
//
// 阅读建议：Ready 证明传输连接可用，不代表某个商品存在，因此启动检查不查询固定商品。

package orderservice

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"
	"google.golang.org/grpc/test/bufconn"
)

// TestConnectInventory 对照真实内存握手与持续拒绝拨号，启动不能把未就绪当成成功。
func TestConnectInventory(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close(); <-finished })
	connection, err := grpc.NewClient("passthrough:///inventory-test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := connectInventory(ctx, connection); err != nil || connection.GetState() != connectivity.Ready {
		t.Fatalf("未等到 Ready：%v", err)
	}
	// 这个拨号器永远拒绝，等待函数只能依靠启动预算结束，不会无限等待。
	unavailable, err := grpc.NewClient("passthrough:///unavailable-test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return nil, errors.New("测试拒绝拨号") }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unavailable.Close() })
	short, stop := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer stop()
	if err := connectInventory(short, unavailable); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("启动预算没有生效：%v", err)
	}
}

// TestInventoryIdleReconnect 不只测试首次 Connect，还让连接真正进入 Idle 后再发一次 RPC。
// 解析器收到的标志直接决定是否读取 TXT；删掉生产选项时，这个测试必须失败。
func TestInventoryIdleReconnect(t *testing.T) {
	connection, builds := memoryInventoryConnection(t)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second) // 测试异常时也要退出等待。
	defer cancel()
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := healthv1.NewHealthClient(connection).Check(ctx, &healthv1.HealthCheckRequest{}); err != nil {
			t.Fatal(err)
		}
		select {
		case disabled := <-builds:
			if !disabled {
				t.Fatal("解析器仍会读取 DNS 服务配置")
			}
		case <-ctx.Done():
			t.Fatal("没有观察到解析器重建")
		}
		if attempt == 0 {
			// 观察真实状态，不靠固定 sleep 猜测连接是否已经被回收。
			for state := connection.GetState(); state != connectivity.Idle; state = connection.GetState() {
				if !connection.WaitForStateChange(ctx, state) {
					t.Fatal("连接没有进入 Idle")
				}
			}
		}
	}
}

// memoryInventoryConnection 把测试接线单独放好，让主测试只关注“调用、空闲、再次调用”。
func memoryInventoryConnection(t *testing.T) (*grpc.ClientConn, <-chan bool) {
	t.Helper()
	listener := bufconn.Listen(1 << 20) // 足够容纳健康检查的小报文，不监听系统端口。
	server := grpc.NewServer()
	healthv1.RegisterHealthServer(server, health.NewServer())
	finished := make(chan error, 1) // Serve 只返回一次，清理时读回，确认后台任务已经退出。
	go func() { finished <- server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close(); <-finished })
	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	runtime := &telemetry.Runtime{Metrics: telemetry.NewMetrics("idle-test"), Provider: provider, Propagator: propagation.TraceContext{}}
	builder := manual.NewBuilderWithScheme("inventory-memory")
	builder.InitialState(resolver.State{Addresses: []resolver.Address{{Addr: "memory"}}})
	builds := make(chan bool, 2) // 首次解析和空闲后重建各写一次；主测试依次读取。
	builder.BuildCallback = func(_ resolver.Target, _ resolver.ClientConn, options resolver.BuildOptions) {
		builds <- options.DisableServiceConfig
	}
	const testIdleTimeout = 50 * time.Millisecond // 缩短等待，但仍走 grpc-go 的真实空闲管理器。
	options := inventoryDialOptions(testIdleTimeout, runtime)
	options = append(options, grpc.WithResolvers(builder), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }))
	connection, err := grpc.NewClient("inventory-memory:///stock", options...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return connection, builds
}
