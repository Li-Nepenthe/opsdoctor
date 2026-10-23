// 本文件：用内存 OTLP 服务验证导出连接的 DNS 选项和空闲恢复。
//
// 即使没有真实 Jaeger，也能检查导出器是否把选项传给底层 grpc-go。
// 内存解析器只提供地址，并报告是否收到禁用服务配置的要求。
//
// 主要内容：
//   - TestExporterIdleReconnect：导出一条 span，等连接回收，再导出另一条。
//   - traceReceiver.Export：统计实际收到的 span，防止只创建导出器却没有发送数据。
//   - memoryTraceExporter：准备内存后端、解析事件和退出时的清理。
//
// 阅读建议：先看两次 ExportSpans，再看 CloseCallback 如何证明连接真的经历过空闲回收。

package telemetry

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"
	"google.golang.org/grpc/test/bufconn"
)

// traceReceiver 只统计收到的记录；RPC goroutine 写、测试读，用原子计数避免竞争。
type traceReceiver struct {
	collector.UnimplementedTraceServiceServer              // 未覆盖的方法沿用协议生成代码的默认行为。
	spans                                     atomic.Int32 // 服务端真正收到的 span 数，不是本地队列长度。
}

// Export 模拟后端确认接收，允许测试验证恢复后的记录确实跨过了 gRPC 连接。
func (receiver *traceReceiver) Export(_ context.Context, request *collector.ExportTraceServiceRequest) (*collector.ExportTraceServiceResponse, error) {
	for _, resource := range request.ResourceSpans {
		for _, scope := range resource.ScopeSpans {
			receiver.spans.Add(int32(len(scope.Spans)))
		}
	}
	return &collector.ExportTraceServiceResponse{}, nil
}

// TestExporterIdleReconnect 在首次连接和空闲重建时都检查解析选项，还验证两次实际发送。
func TestExporterIdleReconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second) // 每一步异常都能有限退出。
	defer cancel()
	exporter, receiver, builds, closed := memoryTraceExporter(t, ctx)
	for attempt := 0; attempt < 2; attempt++ {
		span := tracetest.SpanStub{Name: "idle-export"}.Snapshot()
		if err := exporter.ExportSpans(ctx, []sdktrace.ReadOnlySpan{span}); err != nil {
			t.Fatal(err)
		}
		select {
		case disabled := <-builds:
			if !disabled {
				t.Fatal("OTLP 解析器仍会读取 DNS 服务配置")
			}
		case <-ctx.Done():
			t.Fatal("没有观察到 OTLP 解析器重建")
		}
		if attempt == 0 {
			// 等明确的回收通知，不用 sleep 猜测是否已经进入 Idle。
			select {
			case <-closed:
			case <-ctx.Done():
				t.Fatal("OTLP 连接没有被空闲回收")
			}
		}
	}
	if receiver.spans.Load() != 2 {
		t.Fatalf("实际接收 span=%d", receiver.spans.Load())
	}
}

// memoryTraceExporter 只准备内存后端和解析器，回收和重建的断言留在主测试里。
func memoryTraceExporter(t *testing.T, ctx context.Context) (sdktrace.SpanExporter, *traceReceiver, <-chan bool, <-chan struct{}) {
	t.Helper()
	listener := bufconn.Listen(1 << 20) // 测试只传两条小记录，不建立真实网络连接。
	server := grpc.NewServer()
	receiver := &traceReceiver{}
	collector.RegisterTraceServiceServer(server, receiver)
	finished := make(chan error, 1) // 清理时必须等 Serve 返回，不能留下后台服务。
	go func() { finished <- server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close(); <-finished })
	builder := manual.NewBuilderWithScheme("dns") // 只替换本次连接的 DNS 解析器，不注册全局对象。
	builder.InitialState(resolver.State{Addresses: []resolver.Address{{Addr: "memory"}}})
	builds := make(chan bool, 2)     // 两次解析分别交给主测试核对。
	closed := make(chan struct{}, 2) // 空闲回收和最终关闭各通知一次。
	builder.BuildCallback = func(_ resolver.Target, _ resolver.ClientConn, options resolver.BuildOptions) {
		builds <- options.DisableServiceConfig
	}
	builder.CloseCallback = func() { closed <- struct{}{} }
	const testIdleTimeout = 50 * time.Millisecond // 只缩短测试等待，仍使用真实的空闲管理流程。
	exporter, err := newTraceExporter(ctx, "http://collector-memory:4317", grpc.WithResolvers(builder), grpc.WithIdleTimeout(testIdleTimeout),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = exporter.Shutdown(context.Background()) })
	return exporter, receiver, builds, closed
}
