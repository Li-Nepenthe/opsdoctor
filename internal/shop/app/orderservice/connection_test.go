// 本文件：验证订单服务在首次库存连接就绪后才开放入口。
//
// 用 gRPC 的内存连接进行握手，不监听真实网络端口。失败测试直接返回拨号错误。
//
// 主要内容：
//   - TestConnectInventory：检查握手成功和启动预算耗尽两条路径。
//
// 阅读建议：Ready 证明传输连接可用，不代表某个商品存在，因此启动检查不查询固定商品。

package orderservice

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
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
