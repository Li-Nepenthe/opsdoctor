// 本文件：按结构化条件搜索一个系统的 Loki 日志。
//
// 调用者不能直接写 LogQL。服务、级别和关键词经过字符串转义后，由这里拼成
// 固定形状的查询。关键词里的引号或管道符因此只是要查的文字。
//
// 主要内容：
//   - Search：检查范围、拼接查询、整理日志行。
//   - expression：统一使用精确服务匹配和普通文本筛选。
//
// 阅读建议：先看 expression 中的 strconv.Quote，再看 Search 的窗口和条数限制。

package loki

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source/httpbackend"
)

// Client 绑定一个日志后端，工具不能切换租户头或服务器地址。
type Client struct{ backend *httpbackend.Client }

// New 复用有界 HTTP 客户端，日志后端故障不能让请求无限等待。
func New(address string) (*Client, error) {
	backend, err := httpbackend.New(address)
	if err != nil {
		return nil, err
	}
	return &Client{backend: backend}, nil
}

// Close 在停机时释放空闲连接。
func (client *Client) Close() { client.backend.Close() }

// Search 不使用调用者的字符串作为完整查询，且不会请求 Loki 的管理或写入接口。
func (client *Client) Search(ctx context.Context, query source.LogQuery) ([]source.LogEntry, error) {
	if query.Service == "" || query.Limit < 1 || query.Limit > 100 || query.End.Before(query.Start) || query.End.Sub(query.Start) > time.Hour {
		return nil, fmt.Errorf("日志查询条件超过限制")
	}
	values := url.Values{"query": {expression(query)}, "start": {strconv.FormatInt(query.Start.UnixNano(), 10)}, "end": {strconv.FormatInt(query.End.UnixNano(), 10)}, "limit": {strconv.Itoa(query.Limit)}, "direction": {"backward"}}
	var response struct {
		Status string // HTTP 200 并不保证查询成功。
		Data   struct {
			ResultType string // 只接受日志流，不接受指标查询结果。
			Result     []struct {
				Stream map[string]string // 服务来自实际日志标签。
				Values [][]string        // 每项为时间戳和日志正文。
			}
		}
	}
	if err := client.backend.Get(ctx, "/loki/api/v1/query_range", values, &response); err != nil {
		return nil, err
	}
	if response.Status != "success" || response.Data.ResultType != "streams" {
		return nil, fmt.Errorf("日志后端 Loki 未返回日志流")
	}
	entries := []source.LogEntry{}
	for _, stream := range response.Data.Result {
		// 后端若返回另一个服务，直接报错，不能把混入的证据当作当前服务的日志。
		if stream.Stream["service"] != query.Service {
			return nil, fmt.Errorf("日志后端 Loki 日志服务与查询不一致")
		}
		for _, pair := range stream.Values {
			if len(pair) != 2 {
				return nil, fmt.Errorf("日志后端 Loki 日志行格式不合法")
			}
			if _, err := strconv.ParseInt(pair[0], 10, 64); err != nil {
				return nil, fmt.Errorf("日志后端 Loki 时间戳不合法")
			}
			entries = append(entries, source.LogEntry{Time: pair[0], Service: query.Service, Line: pair[1]})
		}
	}
	// 不同流的返回顺序不等于全局时间顺序，合并后再取最近的若干条。
	sort.SliceStable(entries, func(i, j int) bool {
		a, _ := strconv.ParseInt(entries[i].Time, 10, 64)
		b, _ := strconv.ParseInt(entries[j].Time, 10, 64)
		return a > b
	})
	return entries[:min(query.Limit, len(entries))], nil
}

// expression 用 Quote 把用户文字放进字符串字面量，不能让它变成查询运算符。
func expression(query source.LogQuery) string {
	text := "{service=" + strconv.Quote(query.Service) + "}"
	if query.Keyword != "" {
		text += " |= " + strconv.Quote(query.Keyword)
	}
	if query.Level != "" || query.TraceID != "" {
		text += " | json"
	}
	if query.Level != "" {
		text += " | level=" + strconv.Quote(query.Level)
	}
	if query.TraceID != "" {
		text += " | trace_id=" + strconv.Quote(query.TraceID)
	}
	return text
}
