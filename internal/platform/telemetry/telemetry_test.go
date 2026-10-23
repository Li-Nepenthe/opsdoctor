// 本文件：不用监控后端验证指标和跨 Kafka 的关联信息。
//
// 内存里的 span 导出器能让测试检查真实父子关系，而不需要等 Jaeger 启动。
// HTTP 测试走真实 Gin 中间件，再读取注册表，能发现漏记和错误的标签。
//
// 主要内容：
//   - TestKafkaPropagation：发送、提取后保持 trace ID，并把生产 span 作为消费 span 的父节点。
//   - TestHTTPMetrics：检查模板路由、未知路径、限流和恢复后的指标。
//   - TestRPCMetrics：检查客户端和服务端状态，包括本地超时。
//
// 阅读建议：测试只用内存，不发送 OTLP 数据，也不改 OTel 的全局 Provider。

package telemetry

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/httpx"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestKafkaPropagation 不只比较字符串，还检查消费 span 的远端父节点确实是生产 span。
func TestKafkaPropagation(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	ctx, producer := provider.Tracer("test").Start(t.Context(), "publish")
	headers := KafkaHeaders{}
	headers.Set("TraceParent", "old-value")
	propagator := propagation.TraceContext{}
	propagator.Inject(ctx, &headers)
	extracted := propagator.Extract(context.Background(), &headers)
	parent := trace.SpanContextFromContext(extracted)
	if parent.TraceID() != producer.SpanContext().TraceID() || parent.SpanID() != producer.SpanContext().SpanID() || !parent.IsRemote() {
		t.Fatal("跨消息传播没有保留远端父节点")
	}
	_, consumer := provider.Tracer("test").Start(extracted, "process")
	consumer.End()
	producer.End()
	spans := exporter.GetSpans()
	if len(headers.Keys()) != 1 || len(spans) != 2 || spans[0].Parent.SpanID() != producer.SpanContext().SpanID() {
		t.Fatalf("旧头未替换或父子关系不正确：headers=%v spans=%v", headers.Keys(), spans)
	}
	// 无效外部头应被忽略，不能伪造一个有效父节点，也不应导致处理消息崩溃。
	headers.Set("traceparent", "broken")
	if trace.SpanContextFromContext(propagator.Extract(context.Background(), &headers)).IsValid() {
		t.Fatal("无效 traceparent 被接受")
	}
}

// TestHTTPMetrics 让两个不同订单编号归入同一个模板，证明标签不会随业务数据增长。
func TestHTTPMetrics(t *testing.T) {
	metrics := NewMetrics("order-test")
	router := gin.New()
	router.Use(metrics.HTTPMiddleware(), httpx.Recovery(slog.New(slog.NewTextHandler(io.Discard, nil))))
	router.GET("/orders/:id", func(ctx *gin.Context) { ctx.Status(http.StatusOK) })
	router.GET("/limited", func(ctx *gin.Context) { ctx.AbortWithStatus(http.StatusTooManyRequests) })
	router.GET("/panic", func(*gin.Context) { panic("expected") })
	for _, path := range []string{"/orders/1", "/orders/2", "/missing-a", "/missing-b", "/limited", "/panic"} {
		// 正常请求、未知路由、入口拒绝和 panic 恢复都必须经过同一套计时。
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, family := range families {
		if family.GetName() != "http_server_request_duration_seconds" {
			continue
		}
		found = true
		if len(family.Metric) != 4 {
			t.Fatalf("模板被拆散：%v", family)
		}
		for _, sample := range family.Metric {
			for _, label := range sample.Label {
				if label.GetName() == "route" && (label.GetValue() == "/orders/:id" || label.GetValue() == "unmatched") && sample.GetHistogram().GetSampleCount() != 2 {
					t.Fatalf("模板请求数量不对：%v", sample)
				}
			}
		}
	}
	if !found {
		t.Fatal("HTTP 耗时没有注册或没有写入样本")
	}
	if testutil.ToFloat64(metrics.RateRejected.WithLabelValues("/limited")) != 1 {
		t.Fatal("429 未计数")
	}
}

// TestRPCMetrics 用正常、业务拒绝和本地超时覆盖容易混淆的状态分类。
func TestRPCMetrics(t *testing.T) {
	metrics := NewMetrics("rpc-test")
	for _, testcase := range []struct {
		err  error  // 回调交给拦截器的结果。
		code string // 期望写入的有限状态标签。
	}{
		{nil, "OK"}, // 正常完成不能被当成 Unknown。
		{status.Error(codes.FailedPrecondition, "库存不足"), "FailedPrecondition"}, // 业务拒绝保留原状态。
		{context.DeadlineExceeded, "DeadlineExceeded"},                         // 本地 context 错误也属于超时。
	} {
		_, err := metrics.Server()(t.Context(), nil, &grpc.UnaryServerInfo{FullMethod: "/test/Get"}, func(context.Context, any) (any, error) { return nil, testcase.err })
		if err != testcase.err || rpcCode(err) != testcase.code {
			t.Fatalf("状态被改变：%v", err)
		}
		err = metrics.Client("inventory")(t.Context(), "/test/Get", nil, nil, nil, func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			return testcase.err
		})
		if err != testcase.err {
			t.Fatal("客户端指标改变了返回错误")
		}
	}
	// 注册表应包含两类直方图，各有三种状态，而不是只创建了空的指标定义。
	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, family := range families {
		if family.GetName() == "rpc_client_duration_seconds" || family.GetName() == "rpc_server_duration_seconds" {
			found++
			if len(family.Metric) != 3 {
				t.Fatalf("RPC 状态计数不完整：%v", family)
			}
		}
	}
	if found != 2 {
		t.Fatal("客户端或服务端直方图缺少样本")
	}
}
