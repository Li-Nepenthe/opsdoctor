// 本文件：将服务日志写成 JSON，并把同一请求和调用链的日志关联起来。
// 从 New 读到 Handle，能看到日志写出时怎样取得正在执行的操作标识。

package logging

import (
	"context"
	"io"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// 使用包内专用键，避免别的包意外覆盖这些标识。
type contextKey struct{}

// Fields 保存随请求传递的关联标识，缺少的标识不会出现在日志中。
type Fields struct {
	RequestID string
	TraceID   string
	SpanID    string
}

// WithFields 将标识随上下文传给后续调用，让它们的日志能关联到同一个请求。
func WithFields(ctx context.Context, fields Fields) context.Context {
	return context.WithValue(ctx, contextKey{}, fields)
}

// FromContext 读取请求标识。如果存在有效的当前 span，则使用它的调用链标识。
func FromContext(ctx context.Context) Fields {
	fields, _ := ctx.Value(contextKey{}).(Fields)
	// SQL 或 RPC 可能在子 span 中写日志。每次写出时重新取当前 span，才能定位到实际执行的操作。
	span := trace.SpanContextFromContext(ctx)
	if span.IsValid() {
		fields.TraceID, fields.SpanID = span.TraceID().String(), span.SpanID().String()
	}
	return fields
}

// handler 将请求标识留在 JSON 顶层，业务字段则按调用方指定的组写入。
type handler struct {
	next       slog.Handler
	attributes []slog.Attr
	groups     []string
}

// New 创建带固定服务名的 JSON 日志器，顶层时间统一使用 UTC，便于跨服务对齐记录。
// 写入慢时调用方也会等待，这样输出端的积压就不会藏在后台队列里。
func New(writer io.Writer, service string, level slog.Level) *slog.Logger {
	jsonHandler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: utcTime,
	})
	return slog.New(handler{next: jsonHandler.WithAttrs([]slog.Attr{slog.String("service", service)})})
}

// 只调整顶层日志时间，业务组里也可能有名为 time 的字段。
func utcTime(groups []string, attribute slog.Attr) slog.Attr {
	if len(groups) == 0 && attribute.Key == slog.TimeKey && attribute.Value.Kind() == slog.KindTime {
		return slog.String(slog.TimeKey, attribute.Value.Time().UTC().Format(time.RFC3339Nano))
	}
	return attribute
}

// Enabled 沿用输出日志器的级别过滤。
func (wrapped handler) Enabled(ctx context.Context, level slog.Level) bool {
	return wrapped.next.Enabled(ctx, level)
}

// Handle 将业务字段与当前请求的标识写入同一条记录。
func (wrapped handler) Handle(ctx context.Context, record slog.Record) error {
	copyRecord := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	// 同一日志器可能同时写多条记录。本次追加的业务字段不能改动其他记录共用的固定字段。
	attributes := append([]slog.Attr(nil), wrapped.attributes...)
	var current []slog.Attr
	record.Attrs(func(attribute slog.Attr) bool {
		current = append(current, attribute)
		return true
	})
	attributes = append(attributes, groupAttributes(wrapped.groups, current)...)
	copyRecord.AddAttrs(mergeGroups(attributes)...)
	fields := FromContext(ctx)
	if fields.RequestID != "" {
		copyRecord.AddAttrs(slog.String("request_id", fields.RequestID))
	}
	if fields.TraceID != "" {
		copyRecord.AddAttrs(slog.String("trace_id", fields.TraceID))
	}
	if fields.SpanID != "" {
		copyRecord.AddAttrs(slog.String("span_id", fields.SpanID))
	}
	return wrapped.next.Handle(ctx, copyRecord)
}

// WithAttrs 复制已有的固定字段再添加新字段，避免派生的日志器相互改动。
func (wrapped handler) WithAttrs(attributes []slog.Attr) slog.Handler {
	copyAttributes := append([]slog.Attr(nil), wrapped.attributes...)
	copyAttributes = append(copyAttributes, groupAttributes(wrapped.groups, attributes)...)
	return handler{next: wrapped.next, attributes: copyAttributes, groups: wrapped.groups}
}

// WithGroup 为后续业务字段保留分组，关联标识始终留在顶层。
func (wrapped handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return wrapped
	}
	groups := append([]string(nil), wrapped.groups...)
	groups = append(groups, name)
	return handler{next: wrapped.next, attributes: wrapped.attributes, groups: groups}
}

func groupAttributes(groups []string, attributes []slog.Attr) []slog.Attr {
	attributes = append([]slog.Attr(nil), attributes...)
	for index := len(groups) - 1; index >= 0; index-- {
		attributes = []slog.Attr{slog.GroupAttrs(groups[index], attributes...)}
	}
	return attributes
}

func mergeGroups(attributes []slog.Attr) []slog.Attr {
	var merged []slog.Attr
	positions := make(map[string]int)
	for _, attribute := range attributes {
		if attribute.Value.Kind() != slog.KindGroup {
			merged = append(merged, attribute)
			continue
		}
		// 固定字段和这次新增的字段可能属于同一组。保留组原来的位置并合并内容，避免同名 JSON 键被覆盖。
		if index, exists := positions[attribute.Key]; exists {
			children := append([]slog.Attr(nil), merged[index].Value.Group()...)
			children = append(children, attribute.Value.Group()...)
			merged[index] = slog.GroupAttrs(attribute.Key, mergeGroups(children)...)
			continue
		}
		positions[attribute.Key] = len(merged)
		merged = append(merged, slog.GroupAttrs(attribute.Key, mergeGroups(attribute.Value.Group())...))
	}
	return merged
}
