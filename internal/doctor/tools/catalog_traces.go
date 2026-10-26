// 本文件：登记慢请求搜索与单条调用链树。
//
// 工具只捕获传入的 source 接口，调用时不能更换后端地址或账号。
//
// 主要内容：addTraces 登记本文件的工具，字段限制与运行时校验共用一份定义。
// 阅读建议：先用搜索找 ID，再用详情把同一次请求的父子步骤接起来。

package tools

import (
	"context"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"time"
)

// addTraces 先搜索慢请求拿到 ID，再查同一条链，避免把不同请求的步骤拼成一个故事。
func (registry *Registry) addTraces(backend source.TraceSource) {
	// 搜索只挑慢请求；想看完整结构时必须提供同一次请求的 trace ID。
	registry.add("search_traces", "按服务查慢请求，每条只展示最慢五个 span；可按 trace ID 继续查。", map[string]Property{
		"service": nameProperty("服务名"), "operation": textProperty("可选操作名，如 POST /orders", 0, 160),
		"min_duration_ms": integerProperty("请求最小耗时，毫秒", 0, 60000, 200), "lookback_seconds": integerProperty("回看秒数", 1, 3600, 300), "limit": integerProperty("最多返回多少条调用链", 1, 10, 3),
	}, []string{"service"}, func(ctx context.Context, args arguments) (output, error) {
		end := time.Now().UTC()
		traces, err := backend.Search(ctx, source.TraceQuery{Service: args.text("service"), Operation: args.text("operation"), Start: end.Add(-time.Duration(args.integer("lookback_seconds")) * time.Second), End: end, MinDuration: time.Duration(args.integer("min_duration_ms")) * time.Millisecond, Limit: args.integer("limit")})
		return summarizeTraces(traces), err
	})
	id := textProperty("十六或三十二位小写十六进制 trace ID", 16, 32)
	id.Pattern = `^(?:[0-9a-f]{16}|[0-9a-f]{32})$`
	registry.add("get_trace", "查看一条调用链的树形摘要；缺失父节点会明确标记。", map[string]Property{"trace_id": id}, []string{"trace_id"}, func(ctx context.Context, args arguments) (output, error) {
		trace, err := backend.Get(ctx, args.text("trace_id"))
		return summarizeTree(trace), err
	})
}
