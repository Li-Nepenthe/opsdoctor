// 本文件：向配置中的日志索引发送有界的只读搜索。
//
// OpenSearch 的查询 DSL 是描述查询的 JSON。这里用 Go 的 map 组织固定结构，再让
// json.Marshal 转义值；不会把关键词拼进 query_string 或 URL 路径。
//
// 主要内容：New 固定客户端边界；expression 翻译全部条件；Search 查询并整理结果。
// 阅读建议：先看 expression 的 filter 和 must，再看 search 怎样限制 HTTP 响应。

package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source/httpbackend"
)

// 查询限制与其他日志适配器一致；HTTP 连接数与医生一次并行查证的数量相同。
const (
	maximumLogs        = 100              // 一次返回太多日志会挤占模型的证据窗口。
	maximumWindow      = time.Hour        // 只查近期证据，避免全索引扫描。
	maximumConnections = 4                // 对应医生单次并行工具调用上限。
	headerTimeout      = 5 * time.Second  // 总预算的一半留给读取和解析响应。
	requestTimeout     = 10 * time.Second // 与只读工具的单次时间预算一致。
)

// Client 在构造时绑定一个索引，Search 参数没有地址或索引字段。
type Client struct {
	config Config       // 只由 New 写入，查询期间不修改。
	http   *http.Client // 可并发查询；客户端和请求上下文共同限制等待时间。
}

// New 不探测后端，单元测试和配置校验不会因此访问外部服务。
func New(config Config) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxConnsPerHost = maximumConnections
	transport.ResponseHeaderTimeout = headerTimeout
	return &Client{config: config, http: &http.Client{Transport: transport, Timeout: requestTimeout,
		// 不跟随重定向，避免配置后端把账号或请求带到别的地址。
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

// Close 在停机和测试结束时归还连接，不等待垃圾回收。
func (client *Client) Close() { client.http.CloseIdleConnections() }

// Search 与 Loki 接受相同的范围和条数；后端不能让调用方绕过这些上限。
func (client *Client) Search(ctx context.Context, query source.LogQuery) ([]source.LogEntry, error) {
	if query.Service == "" || query.Limit < 1 || query.Limit > maximumLogs || query.End.Before(query.Start) || query.End.Sub(query.Start) > maximumWindow {
		return nil, fmt.Errorf("日志查询条件超过限制")
	}
	body, err := json.Marshal(expression(query, client.config.Fields))
	if err != nil {
		return nil, fmt.Errorf("无法编码 OpenSearch 日志条件")
	}
	raw, err := client.search(ctx, body)
	if err != nil {
		return nil, err
	}
	return entries(raw, query, client.config.Fields)
}

// expression 固定查询结构，服务、级别、请求标识用精确匹配，关键词只占一个 JSON 值。
func expression(query source.LogQuery, fields Fields) map[string]any {
	filters := []any{map[string]any{"term": map[string]string{fields.Service: query.Service}},
		map[string]any{"range": map[string]any{fields.Timestamp: map[string]string{"gte": query.Start.UTC().Format(time.RFC3339Nano), "lte": query.End.UTC().Format(time.RFC3339Nano), "format": "strict_date_optional_time_nanos"}}}}
	if query.Level != "" {
		filters = append(filters, map[string]any{"term": map[string]string{fields.Level: query.Level}})
	}
	if query.TraceID != "" {
		filters = append(filters, map[string]any{"term": map[string]string{fields.TraceID: query.TraceID}})
	}
	condition := map[string]any{"filter": filters}
	if query.Keyword != "" {
		condition["must"] = []any{map[string]any{"match_phrase": map[string]string{fields.Message: query.Keyword}}}
	}
	return map[string]any{"size": query.Limit, "track_total_hits": false, "query": map[string]any{"bool": condition}, "sort": []any{map[string]string{fields.Timestamp: "desc"}}}
}

// search 的 POST 只用于 _search 查询，不提供通用写入方法，也不接受调用方路径。
func (client *Client) search(ctx context.Context, body []byte) ([]byte, error) {
	// 地址和索引只取创建时的配置；POST 的唯一目标就是只读的 _search。
	address := strings.TrimRight(client.config.URL, "/") + "/" + client.config.Index + "/_search"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("无法创建 OpenSearch 日志请求")
	}
	request.Header.Set("Content-Type", "application/json")
	if client.config.Username != "" {
		request.SetBasicAuth(client.config.Username, client.config.Password)
	}
	response, err := client.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("OpenSearch 日志请求未完成")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenSearch 日志返回 HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, httpbackend.MaximumBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("OpenSearch 日志响应读取失败")
	}
	if len(raw) > httpbackend.MaximumBytes {
		return nil, fmt.Errorf("OpenSearch 日志响应超过 2MiB，请缩小查询范围")
	}
	return raw, nil
}
