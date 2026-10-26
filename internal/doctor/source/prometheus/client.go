// 本文件：把医生的指标和告警查询转换成 Prometheus HTTP API 请求。
//
// 即时查询返回一个采样点，范围查询返回一条时间线。这里把两种格式统一起来，
// 工具就不用按产品的 JSON 外壳写两套摘要逻辑。
//
// 主要内容：
//   - New、Close：绑定一个系统的 Prometheus 并释放连接。
//   - Query：读取即时或范围查询，限制序列和点数。
//   - Alerts：只保留 firing 告警，不把观察中的 pending 当成已触发。
//
// 阅读建议：先看 Query 的路径选择，再看 point 对后端类型的检查。

package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source/httpbackend"
)

// Client 只持有一个后端，Query 没有临时换地址的入口。
type Client struct{ backend *httpbackend.Client }

// New 在启动时检查地址；它不需要先查一次数据才能建立只读客户端。
func New(address string) (*Client, error) {
	backend, err := httpbackend.New(address)
	if err != nil {
		return nil, err
	}
	return &Client{backend: backend}, nil
}

// Close 关闭空闲连接，方便整体停止医生时释放各系统的资源。
func (client *Client) Close() { client.backend.Close() }

// Query 同时在后端请求和本地解码限制数量，不能只相信调用方已经限过一次。
func (client *Client) Query(ctx context.Context, query source.MetricQuery) (source.MetricResult, error) {
	path := "/api/v1/query"
	values := url.Values{"query": {query.Expression}, "timeout": {"8s"}, "limit": {"21"}}
	if !query.Start.IsZero() {
		if query.Step < time.Second || query.End.Before(query.Start) || query.End.Sub(query.Start) > time.Hour || query.End.Sub(query.Start)/query.Step > 240 {
			return source.MetricResult{}, fmt.Errorf("指标范围或采样点数超过限制")
		}
		path = "/api/v1/query_range"
		values.Set("start", query.Start.Format(time.RFC3339Nano))
		values.Set("end", query.End.Format(time.RFC3339Nano))
		values.Set("step", strconv.FormatFloat(query.Step.Seconds(), 'f', -1, 64))
	}
	var response struct {
		Status string // success 才能把数据当成有效证据。
		Data   struct {
			ResultType string          // vector、matrix 和 scalar 的外形不同。
			Result     json.RawMessage // 先读类型，再决定怎样解码。
		}
	}
	if err := client.backend.Get(ctx, path, values, &response); err != nil {
		return source.MetricResult{}, err
	}
	if response.Status != "success" {
		return source.MetricResult{}, fmt.Errorf("指标后端 Prometheus 查询失败")
	}
	return decodeResult(response.Data.ResultType, response.Data.Result)
}

// decodeResult 把不同查询结果转成相同结构，同时限制返回的序列和采样点。
func decodeResult(kind string, raw json.RawMessage) (source.MetricResult, error) {
	result := source.MetricResult{Series: []source.Series{}}
	if kind == "scalar" || kind == "string" {
		item, err := point(raw)
		if err != nil {
			return result, err
		}
		result.Series = append(result.Series, source.Series{Labels: map[string]string{}, Points: []source.Point{item}})
		return result, nil
	}
	if kind != "vector" && kind != "matrix" {
		return result, fmt.Errorf("指标后端 Prometheus 返回未知结果类型")
	}
	var rows []struct {
		Metric map[string]string // 一组标签代表一条序列。
		Value  json.RawMessage   // 即时查询只有一个点。
		Values []json.RawMessage // 范围查询有多个点。
	}
	if json.Unmarshal(raw, &rows) != nil {
		return result, fmt.Errorf("指标后端 Prometheus 序列格式不合法")
	}
	result.Limited = len(rows) > 20
	for _, row := range rows[:min(20, len(rows))] {
		points := row.Values
		if kind == "vector" {
			points = []json.RawMessage{row.Value}
		}
		series := source.Series{Labels: row.Metric, Points: []source.Point{}}
		if len(points) > 241 {
			result.Limited = true
		}
		for _, raw := range points[:min(241, len(points))] {
			item, err := point(raw)
			if err != nil {
				return result, err
			}
			series.Points = append(series.Points, item)
		}
		result.Series = append(result.Series, series)
	}
	return result, nil
}

// point 不假设后端一定返回两个元素，坏响应不能造成越界 panic。
func point(raw json.RawMessage) (source.Point, error) {
	var pair []json.RawMessage
	var value source.Point
	if json.Unmarshal(raw, &pair) != nil || len(pair) != 2 || json.Unmarshal(pair[0], &value.Time) != nil || json.Unmarshal(pair[1], &value.Value) != nil {
		return value, fmt.Errorf("指标后端 Prometheus 采样点格式不合法")
	}
	return value, nil
}

// Alerts 读取实际告警状态，不根据某个指标值自行猜测规则是否已经触发。
func (client *Client) Alerts(ctx context.Context) ([]source.Alert, error) {
	var response struct {
		Status string // Prometheus 的业务错误也可能使用 HTTP 200。
		Data   struct {
			Alerts []struct {
				State       string // pending 仍在等待持续时间，不算 firing。
				Labels      map[string]string
				Annotations map[string]string
				ActiveAt    time.Time
				Value       string
			}
		}
	}
	if err := client.backend.Get(ctx, "/api/v1/alerts", nil, &response); err != nil {
		return nil, err
	}
	if response.Status != "success" {
		return nil, fmt.Errorf("指标后端 Prometheus 告警查询失败")
	}
	alerts := []source.Alert{}
	for _, item := range response.Data.Alerts {
		if item.State != "firing" {
			continue
		}
		alerts = append(alerts, source.Alert{Labels: item.Labels, Annotations: item.Annotations, ActiveAt: item.ActiveAt, Value: item.Value})
	}
	return alerts, nil
}
