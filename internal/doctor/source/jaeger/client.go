// 本文件：通过 Jaeger 2.20.0 的 /api/traces 查询和转换调用链。这个内部接口可能随版本变化，升级时要用真实样例检查字段和父子关系。

package jaeger

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source/httpbackend"
)

// Client 始终使用创建时绑定的 Jaeger，trace ID 不参与选择后端。
type Client struct {
	backend *httpbackend.Client // 仍绑定没有凭据、查询串和路径的固定根地址。
	prefix  string              // 部署者提供的安全前缀，工具调用不能改变。
}

// New 在启动时发现地址错误，避免等诊断开始才暴露配置问题。
func New(address string) (*Client, error) {
	base, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("调用链后端 Jaeger 地址格式不合法")
	}
	// 不解码或清理可疑路径：编码、点路径和空段都直接拒绝，避免代理与 Go 解释不同。
	prefix := strings.TrimSuffix(base.Path, "/")
	if base.RawPath != "" || strings.Contains(base.EscapedPath(), "%") || base.RawQuery != "" || base.ForceQuery || strings.Contains(address, "#") {
		return nil, fmt.Errorf("调用链后端 Jaeger 部署地址不能含编码路径、查询串或片段")
	}
	if prefix != "" {
		if !regexp.MustCompile(`^/[A-Za-z0-9._~/-]+$`).MatchString(prefix) || strings.Contains(base.Path, "//") {
			return nil, fmt.Errorf("调用链后端 Jaeger 部署前缀只能包含普通路径段")
		}
		for _, segment := range strings.Split(prefix[1:], "/") {
			if segment == "." || segment == ".." {
				return nil, fmt.Errorf("调用链后端 Jaeger 部署前缀不能包含点路径")
			}
		}
	}
	// 公共客户端的根地址校验原样执行；这里没有给其他适配器增加路径权限。
	base.Path = ""
	backend, err := httpbackend.New(base.String())
	if err != nil {
		return nil, err
	}
	return &Client{backend: backend, prefix: prefix}, nil
}

// Close 释放空闲 HTTP 连接，停机后不继续占用后端资源。
func (client *Client) Close() { client.backend.Close() }

// wireTrace 只解码用得上的字段。Jaeger 的其他内容不会自动出现在工具结果里。
type wireTrace struct {
	TraceID   string
	Processes map[string]struct {
		ServiceName string // span 通过 processID 引用服务名称。
	}
	Spans []struct {
		TraceID       string // 检查 span 是否属于这条调用链。
		SpanID        string
		OperationName string
		ProcessID     string // 对应 Processes 中的键。
		StartTime     int64  // Unix 微秒。
		Duration      int64  // 耗时微秒。
		References    []struct {
			RefType string // CHILD_OF 是当前调用树需要的父子关系。
			TraceID string // 不把另一条 trace 的节点接进当前树。
			SpanID  string
		}
		Tags []wireTag
	}
}

// wireTag 保留后端的 JSON 值，直接转为 float64 会丢失大整数的精度。
type wireTag struct {
	Key   string
	Type  string
	Value json.RawMessage
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
	if err := client.backend.Get(ctx, client.prefix+path, query, &response); err != nil {
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
		value := source.Span{ID: span.SpanID, Service: process.ServiceName, Operation: span.OperationName, Start: span.StartTime, Duration: span.Duration}
		for _, ref := range span.References {
			if ref.RefType == "CHILD_OF" && ref.TraceID == item.TraceID {
				value.ParentID = ref.SpanID
				break
			}
		}
		tags, err := decodeTags(span.Tags)
		if err != nil {
			return trace, err
		}
		value.Tags = tags
		trace.Spans = append(trace.Spans, value)
	}
	return trace, nil
}

// decodeTags 只保留允许送入诊断的字段。数字和布尔值保留原始写法，以便核对状态码和错误标记。
func decodeTags(tags []wireTag) (map[string]string, error) {
	values := make(map[string]string)
	for _, tag := range tags {
		if !tagAllowed(tag.Key) {
			continue
		}
		var text string
		if tag.Type == "string" {
			if json.Unmarshal(tag.Value, &text) != nil {
				return nil, fmt.Errorf("调用链后端 Jaeger 字符串属性不合法")
			}
		} else {
			text = string(tag.Value)
		}
		values[tag.Key] = text
	}
	return values, nil
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
