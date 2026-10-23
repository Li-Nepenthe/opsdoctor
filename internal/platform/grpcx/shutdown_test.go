// 本文件：证明 gRPC 停机超时能取消在途请求。
//
// 用标准 health 协议和内存连接制造一个等待 context 的 handler；started 通知保证请求真的开始后才停机。
// 先看 blockingHealth.Check，再看 TestShutdownDeadline 如何同时回收 RPC 和 Serve goroutine。

package grpcx

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"
)

// blockingHealth 借用标准 health 协议模拟在途 RPC，不引入任何 shop 依赖。
type blockingHealth struct {
	grpc_health_v1.UnimplementedHealthServer               // 嵌入默认实现，只重写测试所需的 Check。
	started                                  chan struct{} // Check 唯一关闭，测试等待开始后才启动停机，避免调度时序让测试空跑。
	finished                                 chan struct{} // Check 退出时唯一关闭，测试据此确认 handler 没有遗留。
}

// Check 一直等到 RPC 被取消；它模仿响应 context 的数据库查询，不包含固定睡眠。
func (server *blockingHealth) Check(ctx context.Context, _ *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	close(server.started)
	<-ctx.Done()
	close(server.finished)
	return nil, ctx.Err()
}

// TestShutdownDeadline 证明预算耗尽后 Stop 会取消在途 RPC，并且 Shutdown 等待自己的 goroutine 退出。
func TestShutdownDeadline(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	handler := &blockingHealth{started: make(chan struct{}), finished: make(chan struct{})}
	grpc_health_v1.RegisterHealthServer(server, handler)
	served := make(chan error, 1) // Serve 写一次、Cleanup 读一次，缓冲保证异常提前返回也不阻塞。
	go func() { served <- server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close(); <-served }) // Stop 已接管监听器关闭，重复 Close 的错误不影响清理。
	connection, err := grpc.NewClient("passthrough:///memory", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Error(err)
		}
	})
	client := grpc_health_v1.NewHealthClient(connection)
	callCtx, cancelCall := context.WithTimeout(t.Context(), 3*time.Second) // 即使停机实现失效，测试 RPC 也有最后的退出期限。
	defer cancelCall()
	called := make(chan error, 1) // RPC goroutine 写一次，测试收齐；无需 close。
	go func() { _, err := client.Check(callCtx, &grpc_health_v1.HealthCheckRequest{}); called <- err }()
	select {
	case <-handler.started:
	case <-callCtx.Done():
		t.Fatal("测试 RPC 未进入服务端")
	}
	cleanup, cancelCleanup := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancelCleanup()
	if err := Shutdown(cleanup, server); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("停机未报告预算耗尽：%v", err)
	}
	<-handler.finished
	if err := <-called; err == nil {
		t.Fatal("被强制终止的 RPC 不应成功")
	}
}
