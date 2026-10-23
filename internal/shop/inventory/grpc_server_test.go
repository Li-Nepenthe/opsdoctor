// 本文件：验证 gRPC 错误契约和内存通道往返。
//
// 先看 TestGRPCErrorMapping 的七类错误，再看 TestGRPCRoundTrip 的序列化与分派；testClient 负责建立和关闭内存连接。
// 真正的数据库与取消传播留给集成测试，而不是用假返回值冒充。

package inventory

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"

	inventoryv1 "github.com/Li-Nepenthe/opsdoctor/gen/inventory/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// TestGRPCErrorMapping 覆盖契约全部错误码；包装后的错误也必须能映射，且不能泄漏底层文字。
func TestGRPCErrorMapping(t *testing.T) {
	cases := []struct {
		name string     // name 说明客户端应如何理解失败。
		err  error      // err 模拟业务或仓储产生的真实错误链。
		code codes.Code // code 是对外承诺的 gRPC 状态。
	}{
		{"参数错误", ErrInvalidArgument, codes.InvalidArgument},            // 请求不合法，无需重试。
		{"商品不存在", ErrNotFound, codes.NotFound},                         // 与库存不足区分开。
		{"库存不足", ErrInsufficientStock, codes.FailedPrecondition},       // 库存条件不满足。
		{"调用方超时", context.DeadlineExceeded, codes.DeadlineExceeded},    // 保留超时语义。
		{"主动取消", context.Canceled, codes.Canceled},                     // 主动放弃不能误报内部故障。
		{"数据库暂不可用", ErrUnavailable, codes.Unavailable},                 // 暂时失败，但写操作仍不能盲目重试。
		{"未知内部错误", errors.New("secret-driver-detail"), codes.Internal}, // 防止驱动详情穿透到网络。
	}
	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			server := NewGRPCServer(NewService(&fakeRepository{err: fmt.Errorf("secret-driver-detail：%w", testcase.err)}))
			_, err := server.Deduct(t.Context(), &inventoryv1.DeductRequest{SkuId: 1, Quantity: 1})
			if status.Code(err) != testcase.code || strings.Contains(err.Error(), "secret-driver-detail") {
				t.Fatalf("code=%v err=%v", status.Code(err), err)
			}
		})
	}
}

// TestGRPCRoundTrip 用内存连接运行真实序列化与服务分派；单元测试不监听 TCP、不依赖外部服务。
func TestGRPCRoundTrip(t *testing.T) {
	client := testClient(t, &fakeRepository{})
	stock, err := client.GetStock(t.Context(), &inventoryv1.GetStockRequest{SkuId: 7})
	if err != nil || stock.GetSkuId() != 7 || stock.GetAvailable() != 50 {
		t.Fatalf("查询结果：%v %v", stock, err)
	}
	if _, err := client.Deduct(t.Context(), &inventoryv1.DeductRequest{SkuId: 7, Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Release(t.Context(), &inventoryv1.ReleaseRequest{SkuId: 7, Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	// 空请求经过 protobuf 后字段为 0，仍须由业务校验拒绝。
	if _, err := client.GetStock(t.Context(), &inventoryv1.GetStockRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("空请求：%v", err)
	}
}

// testClient 构造真实 gRPC 内存通道；Cleanup 按后注册先执行的顺序关闭客户端、服务端和监听器。
func testClient(t *testing.T, repository StockRepository) inventoryv1.InventoryServiceClient {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024) // 1 MiB 只用于测试内存传输，足以容纳几个很小的 protobuf 请求。
	server := grpc.NewServer()
	inventoryv1.RegisterInventoryServiceServer(server, NewGRPCServer(NewService(repository)))
	done := make(chan error, 1) // Serve 写一次，Cleanup 读一次；容量 1 避免退出先于清理时阻塞。
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		if err := listener.Close(); err != nil {
			t.Error(err)
		}
		if err := <-done; err != nil && !errors.Is(err, grpc.ErrServerStopped) && !errors.Is(err, io.EOF) {
			t.Error(err)
		}
	})
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
	return inventoryv1.NewInventoryServiceClient(connection)
}
