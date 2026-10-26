// 本文件：读取本项目固定的 Jaeger 2.20.0 查询接口。
//
// 这里使用实测可用的 /api/traces JSON 接口。它是 Jaeger 的内部接口，升级时
// 必须重跑真实样例契约测试，不能因为路径没变就假定字段也没变。
//
// 主要内容：
//   - Search、Get：搜索请求和按 ID 读取完整调用链。
//   - decode：合并 process 与 span，并保留父子关系和诊断属性。
//   - tagAllowed：排除没有必要提供给诊断工具的属性。
//
// 阅读建议：先看 wireTrace 的后端结构，再看 decode 如何转换成通用结构。

package jaeger

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source/httpbackend"
)

// Client 始终使用创建时绑定的 Jaeger，trace ID 不参与选择后端。
type Client struct{ backend *httpbackend.Client }

// New 在启动时发现地址错误，避免等诊断开始才暴露配置问题。
func New(address string) (*Client, error) {
	backend, err := httpbackend.New(address)
	if err != nil {
		return nil, err
	}
	return &Client{backend: backend}, nil
}

// Close 释放空闲 HTTP 连接，停机后不继续占用后端资源。
func (client *Client) Close() { client.backend.Close() }

// wireTrace 只解码用得上的字段。Jaeger 的其他内容不会自动出现在工具结果里。
type wireTrace struct {
	TraceID   string // 整个请求的标识。
	Processes map[string]struct {
		ServiceName string // span 通过 processID 引用服务名称。
	}
	Spans []struct {
		TraceID       string // 检查 span 是否属于这条调用链。
		SpanID        string // 当前步骤标识。
		OperationName string // 原始操作名。
		ProcessID     string // 对应 Processes 中的键。
		StartTime     int64  // Unix 微秒。
		Duration      int64  // 耗时微秒。
		References    []struct {
			RefType string // CHILD_OF 是当前调用树需要的父子关系。
			TraceID string // 不把另一条 trace 的节点接进当前树。
			SpanID  string // 被引用的父节点。
		}
		Tags []struct {
			Key   string          // 根据字段名决定是否需要保留。
			Type  string          // string、bool、int64、float64 等真实 Jaeger 类型。
			Value json.RawMessage // 保留 JSON 数值精度，不先转 float64。
		}
	}
}

// Search 的起止时间和最小耗时分别使用 Jaeger 约定的微秒和时长字符串。
func (client *Client) Search(ctx context.Context, query source.TraceQuery) ([]source.Trace, error) {
	// 适配器再次检查边界，今后其他调用方也不能绕开工具层发起无界搜索。
	if query.Limit < 1 || query.Limit > 10 || query.End.Before(query.Start) || query.End.Sub(query.Start) > time.Hour {
		return nil, fmt.Errorf("调用链查询范围超过限制")
	}
	values := url.Values{
		"service": {query.Service}, "start": {strconv.FormatInt(query.Start.UnixMicro(), 10)},
		"end": {strconv.FormatInt(query.End.UnixMicro(), 10)}, "limit": {strconv.Itoa(query.Limit)},
	}
	if query.Operation != "" {
		values.Set("operation", query.Operation)
	}
	if query.MinDuration > 0 {
		values.Set("minDuration", query.MinDuration.String())
	}
	traces, err := client.read(ctx, "/api/traces", values)
	if err != nil {
		return nil, err
	}
	return traces[:min(query.Limit, len(traces))], nil
}

// Get 只接受十六或三十二位十六进制 ID，不能把路径、主机或查询串塞进这里。
func (client *Client) Get(ctx context.Context, id string) (source.Trace, error) {
	if !regexp.MustCompile(`^(?:[0-9a-f]{16}|[0-9a-f]{32})$`).MatchString(id) {
		return source.Trace{}, fmt.Errorf("trace ID 格式不合法")
	}
	traces, err := client.read(ctx, "/api/traces/"+id, nil)
	if err != nil {
		return source.Trace{}, err
	}
	if len(traces) != 1 || traces[0].ID != id {
		return source.Trace{}, fmt.Errorf("调用链后端 Jaeger 没有返回指定调用链")
	}
	return traces[0], nil
}

// read 同时检查 HTTP 和业务错误。空搜索结果是正常情况，不伪造一条空 trace。
func (client *Client) read(ctx context.Context, path string, query url.Values) ([]source.Trace, error) {
	var response struct {
		Data   []wireTrace       // 搜索和详情共用这个数组外壳。
		Errors []json.RawMessage // 原始错误可能带请求内容，不直接回显。
	}
	if err := client.backend.Get(ctx, path, query, &response); err != nil {
		return nil, err
	}
	if len(response.Errors) > 0 {
		return nil, fmt.Errorf("调用链后端 Jaeger 查询返回错误")
	}
	traces := make([]source.Trace, 0, len(response.Data))
	for _, item := range response.Data {
		trace, err := decode(item)
		if err != nil {
			return nil, err
		}
		traces = append(traces, trace)
	}
	return traces, nil
}

// decode 不把 process ID 当成服务名，也不把 FOLLOWS_FROM 误当成树的直接父节点。
func decode(item wireTrace) (source.Trace, error) {
	trace := source.Trace{ID: item.TraceID, Spans: []source.Span{}}
	if item.TraceID == "" {
		return trace, fmt.Errorf("调用链后端 Jaeger 调用链缺少 ID")
	}
	seen := make(map[string]bool)
	for _, span := range item.Spans {
		process, ok := item.Processes[span.ProcessID]
		if !ok || span.SpanID == "" || span.TraceID != item.TraceID || span.Duration < 0 || seen[span.SpanID] {
			return trace, fmt.Errorf("调用链后端 Jaeger span 关系或字段不合法")
		}
		seen[span.SpanID] = true
		value := source.Span{ID: span.SpanID, Service: process.ServiceName, Operation: span.OperationName, Start: span.StartTime, Duration: span.Duration, Tags: map[string]string{}}
		for _, ref := range span.References {
			if ref.RefType == "CHILD_OF" && ref.TraceID == item.TraceID {
				value.ParentID = ref.SpanID
				break
			}
		}
		for _, tag := range span.Tags {
			if !tagAllowed(tag.Key) {
				continue
			}
			var text string
			if tag.Type == "string" {
				if json.Unmarshal(tag.Value, &text) != nil {
					return trace, fmt.Errorf("调用链后端 Jaeger 字符串属性不合法")
				}
			} else {
				// 数字和布尔值也能说明错误或状态，不能因为它们不是字符串就丢掉。
				text = string(tag.Value)
			}
			value.Tags[tag.Key] = text
		}
		trace.Spans = append(trace.Spans, value)
	}
	return trace, nil
}

// tagAllowed 只保留定位服务、SQL 模板、RPC 和错误状态所需的字段。
func tagAllowed(key string) bool {
	switch key {
	case "db.system", "db.system.name", "db.name", "db.namespace", "db.statement", "db.query.text", "db.operation.name",
		"rpc.service", "rpc.method", "rpc.grpc.status_code", "rpc.response.status_code", "error", "otel.status_code", "http.route", "http.request.method", "http.response.status_code", "messaging.destination.name":
		return true
	default:
		return false
	}
}
