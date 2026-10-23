// 本文件：创建进程自己的调用链提供者，并把 span 批量送到 Jaeger。
//
// span 可以理解成一次操作的计时记录；trace 把这些记录连成一次请求的完整路径。
// app 持有 Runtime，并在入口、数据库和消息客户端关闭后，最后排空还没发出的记录。
//
// 主要内容：
//   - Runtime、New：读取标准 OTel 环境变量，创建指标和调用链组件。
//   - Shutdown：在有限时间内发送剩余 span，不沿用已取消的进程上下文。
//   - Finish：给手动创建的 span 标注成功或失败，再结束计时。
//
// 阅读建议：没有 OTLP 地址时仍能生成关联 ID，但不向网络发送数据，适合宿主机调试。

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
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint != "" {
		// 错误只指出变量名，不能把可能含口令的 URL 原文写到日志。
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, fmt.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT 必须是不含身份信息的 HTTP 或 HTTPS 地址")
		}
		exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpointURL(endpoint), otlptracegrpc.WithTimeout(exportTimeout))
		if err != nil {
			return nil, fmt.Errorf("初始化 OTLP 导出器失败")
		}
		// SDK 管理队列与发送 goroutine；Shutdown 会停止它们。队列满时舍弃追踪记录，不阻塞订单。
		options = append(options, sdktrace.WithBatcher(exporter, sdktrace.WithExportTimeout(exportTimeout)))
	}
	return &Runtime{Service: service, Metrics: NewMetrics(service), Provider: sdktrace.NewTracerProvider(options...), Propagator: propagation.TraceContext{}}, nil
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
