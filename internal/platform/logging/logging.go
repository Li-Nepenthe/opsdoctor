// 本文件：在 JSON 日志中自动关联请求与服务。
//
// 调用方只需把关联信息放进 context，handler 在写日志时补齐字段。
// 先读 Fields、WithFields 和 New，再看 Handle；WithAttrs/WithGroup 复制字段，是为了避免派生日志器互相改数据。

package logging

import (
	"context"
	"io"
	"log/slog"
	"time"
)

// contextKey 是本包专用的键类型，防止其他包用相同字符串覆盖日志上下文。
type contextKey struct{}

// Fields 是一次请求的关联信息；字符串为空表示当前上下文没有该项。
// 它按值放入 context，不共享可写的 map，因此无需额外的锁。
type Fields struct {
	RequestID string // RequestID 是 HTTP 请求标识，和响应中的 request_id 一致。
	TraceID   string // TraceID 是调用链标识；阶段 3 接入追踪时由适配层提供。
	SpanID    string // SpanID 是当前跨度标识；未接入追踪时保持空字符串。
}

// WithFields 把本次请求的关联信息放进派生上下文，让后面的 SQL 或 RPC 日志可以继续使用它。
// context 类似沿调用链传递的请求附加信息，不能用它代替业务参数。
func WithFields(ctx context.Context, fields Fields) context.Context {
	return context.WithValue(ctx, contextKey{}, fields)
}

// FromContext 供日志包装层取出关联信息；启动日志没有请求上下文时，空字段就不输出。
func FromContext(ctx context.Context) Fields {
	// “值, ok := x.(T)”是安全的类型断言：没有 Fields 时得到零值；这里有意忽略 ok，表示关联信息可选。
	fields, _ := ctx.Value(contextKey{}).(Fields)
	return fields
}

// handler 在 JSON Handler 外包一层，避免每个业务日志调用都手动重复 request_id。
// next 在构造后不变；并发写日志的同步由标准库 JSON Handler 负责。
type handler struct {
	next       slog.Handler // next 执行最终的过滤、编码和写出。
	attributes []slog.Attr  // attributes 保存固定业务字段；派生时复制，不共享可变切片。
	groups     []string     // groups 只作用于业务字段，关联标识始终放在 JSON 顶层。
}

// New 创建固定 service 字段的 JSON 日志器；writer 必须非 nil，level 控制最低级别。
// 返回独立 Logger，便于给服务和测试分别指定输出；writer 写得慢时也会拖慢日志调用，所以不在这里另起后台队列隐藏积压。
func New(writer io.Writer, service string, level slog.Level) *slog.Logger {
	jsonHandler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: utcTime,
	})
	return slog.New(handler{next: jsonHandler.WithAttrs([]slog.Attr{slog.String("service", service)})})
}

// utcTime 只规范日志顶层时间，不改业务字段；保证不同机器的记录能按 UTC 对齐。
func utcTime(groups []string, attribute slog.Attr) slog.Attr {
	if len(groups) == 0 && attribute.Key == slog.TimeKey && attribute.Value.Kind() == slog.KindTime {
		// 只改顶层的系统时间，不能误改业务组里恰好也叫 time 的字段。
		return slog.String(slog.TimeKey, attribute.Value.Time().UTC().Format(time.RFC3339Nano))
	}
	return attribute
}

// Enabled 委托下层决定级别是否启用，不提前构造字符串，减少被过滤日志的开销。
func (wrapped handler) Enabled(ctx context.Context, level slog.Level) bool {
	return wrapped.next.Enabled(ctx, level)
}

// Handle 把 context 里的关联字段补进当前记录，再交给 JSON Handler 写出。
// 重新组装 Record 避免修改共享的底层属性存储，遵守 slog.Handler 的并发约定。
func (wrapped handler) Handle(ctx context.Context, record slog.Record) error {
	// 将业务属性放进调用方要求的组，再在顶层添加关联标识，避免分组后无法查询请求。
	copyRecord := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	// 切片赋值只复制描述信息，底层数组可能共享；append 到 nil 切片会复制元素，避免并发日志互相改数据。
	attributes := append([]slog.Attr(nil), wrapped.attributes...)
	var current []slog.Attr
	record.Attrs(func(attribute slog.Attr) bool {
		current = append(current, attribute)
		return true
	})
	attributes = append(attributes, groupAttributes(wrapped.groups, current)...)
	copyRecord.AddAttrs(mergeGroups(attributes)...)
	fields := FromContext(ctx)
	// 没有请求或调用链时不输出对应空字段，让“尚未接入”与“确实有标识”容易区分。
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

// WithAttrs 创建带固定字段的派生 Handler，不修改原对象，因此多个日志器可以安全共享。
func (wrapped handler) WithAttrs(attributes []slog.Attr) slog.Handler {
	// With 应生成独立的日志器；复制已有字段后再追加，不能让子日志器改变父日志器的内容。
	copyAttributes := append([]slog.Attr(nil), wrapped.attributes...)
	copyAttributes = append(copyAttributes, groupAttributes(wrapped.groups, attributes)...)
	return handler{next: wrapped.next, attributes: copyAttributes, groups: wrapped.groups}
}

// WithGroup 记下业务字段的分组路径，等真正写日志时再组合成 JSON；关联字段仍留在顶层。
func (wrapped handler) WithGroup(name string) slog.Handler {
	if name == "" {
		// slog 约定空组名不改变分组，直接保留当前 handler。
		return wrapped
	}
	groups := append([]string(nil), wrapped.groups...)
	groups = append(groups, name)
	return handler{next: wrapped.next, attributes: wrapped.attributes, groups: groups}
}

// groupAttributes 从内到外包装业务属性；不修改传入切片，避免派生日志器影响父日志器。
func groupAttributes(groups []string, attributes []slog.Attr) []slog.Attr {
	attributes = append([]slog.Attr(nil), attributes...)
	for index := len(groups) - 1; index >= 0; index-- {
		// 先包最内层，再包外层，才能让 WithGroup("a").WithGroup("b") 得到 a.b 的嵌套顺序。
		attributes = []slog.Attr{slog.GroupAttrs(groups[index], attributes...)}
	}
	return attributes
}

// mergeGroups 合并固定属性和当前属性中同名的组，避免 JSON 出现重复组键后被解析器覆盖。
// 它只处理本次调用的属性副本；嵌套组递归合并，不修改父日志器的数据。
func mergeGroups(attributes []slog.Attr) []slog.Attr {
	var merged []slog.Attr
	positions := make(map[string]int)
	for _, attribute := range attributes {
		if attribute.Value.Kind() != slog.KindGroup {
			// 普通字段保持原样；这个函数只合并组，不重新定义业务字段的覆盖规则。
			merged = append(merged, attribute)
			continue
		}
		// 记录首次出现的位置，既合并内容，又保留日志字段的原始顺序。
		if index, exists := positions[attribute.Key]; exists {
			// 固定字段和本次日志可能都属于同一个组；合成一个 JSON 对象，避免后一个同名键覆盖前一个。
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
