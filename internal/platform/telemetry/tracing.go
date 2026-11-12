// 本文件：New 为进程配置调用链，Shutdown 在业务请求排空后送出剩余记录。

package telemetry

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

// exportTimeout 让监控后端故障不能无限拖住业务进程的退出。
const exportTimeout = 5 * time.Second

// Runtime 不写 OTel 全局变量，插件都显式接收 Provider，测试之间因此不会串到另一套调用链。
type Runtime struct {
	Service    string                        // 标准 OTEL_SERVICE_NAME 优先，否则使用程序自己的名字。
	Metrics    *Metrics                      // Prometheus 与调用链共享服务名称。
	Provider   *sdktrace.TracerProvider      // 拥有批量发送任务，需要在停机时关闭。
	Propagator propagation.TextMapPropagator // 只传播 W3C traceparent/tracestate，不携带任意业务数据。
}

// New 的采样方式交给 SDK 读取 OTEL_TRACES_SAMPLER 与 OTEL_TRACES_SAMPLER_ARG。
func New(ctx context.Context, service string) (*Runtime, error) {
	if name := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME")); name != "" {
		service = name
	}
	options := []sdktrace.TracerProviderOption{sdktrace.WithResource(resource.NewSchemaless(
		attribute.String("service.name", service), attribute.String("service.version", Version)))}
	// 本机调试可以不设导出地址，此时仍产生可关联的 ID。
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint != "" {
		// 错误只指出变量名，不能把可能含口令的 URL 原文写到日志。
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, fmt.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT 必须是不含身份信息的 HTTP 或 HTTPS 地址")
		}
		exporter, err := newTraceExporter(ctx, endpoint)
		if err != nil {
			return nil, fmt.Errorf("初始化 OTLP 导出器失败")
		}
		// SDK 管理队列与发送 goroutine；Shutdown 会停止它们。队列满时舍弃追踪记录，不阻塞订单。
		options = append(options, sdktrace.WithBatcher(exporter, sdktrace.WithExportTimeout(exportTimeout)))
	}
	return &Runtime{Service: service, Metrics: NewMetrics(service), Provider: sdktrace.NewTracerProvider(options...), Propagator: propagation.TraceContext{}}, nil
}

// newTraceExporter 的额外拨号选项用于注入测试的内存连接；生产路径不需要额外选项。
// 禁用服务配置始终由代码保证，不依赖操作者为每个进程设置环境变量。
func newTraceExporter(ctx context.Context, endpoint string, dialOptions ...grpc.DialOption) (sdktrace.SpanExporter, error) {
	// 导出器只需要 DNS 返回 IP。跳过 Jaeger 的服务配置 TXT 查询，避免它耗尽整批记录的五秒发送预算。
	dialOptions = append(dialOptions, grpc.WithDisableServiceConfig())
	return otlptracegrpc.New(ctx, otlptracegrpc.WithEndpointURL(endpoint),
		otlptracegrpc.WithTimeout(exportTimeout), otlptracegrpc.WithDialOption(dialOptions...))
}

// Shutdown 在业务排空后执行，给最后一批 span 自己的发送预算。
func (runtime *Runtime) Shutdown(parent context.Context) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), exportTimeout)
	defer cancel()
	return runtime.Provider.Shutdown(ctx)
}

// Finish 只记录固定的失败描述，原始错误可能含业务值，不能整段复制进追踪后端。
func Finish(span trace.Span, err error) {
	if err != nil {
		span.SetStatus(codes.Error, "操作未成功")
	}
	span.End()
}
