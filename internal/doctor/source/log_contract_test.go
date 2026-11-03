// 本文件：让两个日志适配器接受同一张行为测试表。
//
// 契约测试关心调用者能依赖什么：相同条件得到相同日志、相同排序和相同取消行为。
// 测试服务器分别检查 LogQL 和查询 DSL，再返回各自的产品格式，上层期望完全共用。
//
// 主要内容：TestLogSourceContract 覆盖所有查询条件；contractRequest 检查翻译；
// contractResponse 仅负责把同一组模拟日志包装成两个产品的响应。

package source_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source/loki"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source/opensearch"
)

// contractCancelTimeout 让本地阻塞后端尽快收到取消；它不用于判断真实后端的性能。
const contractCancelTimeout = 50 * time.Millisecond

// contractFields 故意使用不同于 shop 的字段名，证明第二实现没有写死采集器格式。
func contractFields() opensearch.Fields {
	return opensearch.Fields{Service: "resource.service.name.keyword", Level: "severity.text.keyword", Message: "body", Timestamp: "observedTimestamp", TraceID: "traceId"}
}

// TestLogSourceContract 的每个用例都先跑 Loki，再跑 OpenSearch，期望值没有后端分支。
func TestLogSourceContract(t *testing.T) {
	base := source.LogQuery{Service: "orders", Start: time.Unix(1000, 0), End: time.Unix(1100, 0), Limit: 2}
	old := source.LogEntry{Time: "1001000000000", Service: "orders", Line: "old message"}
	newer := source.LogEntry{Time: "1020000000000", Service: "orders", Line: "new message"}
	for _, testcase := range []struct {
		name    string                 // 同一接口需要保证的行为。
		change  func(*source.LogQuery) // 只改变当前用例关心的条件。
		rows    []source.LogEntry      // 后端夹具，故意按旧到新排列。
		want    []source.LogEntry      // 两个适配器共享的最终结果。
		timeout bool                   // 模拟后端一直不返回，依赖上下文退出。
	}{
		{name: "按服务", rows: []source.LogEntry{old, newer}, want: []source.LogEntry{newer, old}},
		{name: "按级别", change: func(q *source.LogQuery) { q.Level = "ERROR" }, rows: []source.LogEntry{newer}, want: []source.LogEntry{newer}},
		{name: "按关键字", change: func(q *source.LogQuery) { q.Keyword = "new message" }, rows: []source.LogEntry{newer}, want: []source.LogEntry{newer}},
		{name: "关键字特殊字符", change: func(q *source.LogQuery) { q.Keyword = `"\ + && || (x):*` }, rows: []source.LogEntry{}, want: []source.LogEntry{}},
		{name: "按请求标识", change: func(q *source.LogQuery) { q.TraceID = "0123456789abcdef" }, rows: []source.LogEntry{newer}, want: []source.LogEntry{newer}},
		{name: "按时间范围", change: func(q *source.LogQuery) { q.Start = time.Unix(1010, 0); q.End = time.Unix(1030, 0) }, rows: []source.LogEntry{newer}, want: []source.LogEntry{newer}},
		{name: "条数上限", change: func(q *source.LogQuery) { q.Limit = 1 }, rows: []source.LogEntry{old, newer}, want: []source.LogEntry{newer}},
		{name: "空结果", rows: []source.LogEntry{}, want: []source.LogEntry{}},
		{name: "全部条件", change: func(q *source.LogQuery) { q.Level = "WARN"; q.Keyword = "message"; q.TraceID = "0123456789abcdef" }, rows: []source.LogEntry{newer}, want: []source.LogEntry{newer}},
		{name: "后端超时", timeout: true},
	} {
		for _, backend := range []string{"loki", "opensearch"} {
			t.Run(testcase.name+"/"+backend, func(t *testing.T) {
				query := base
				if testcase.change != nil {
					testcase.change(&query)
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					contractRequest(t, r, backend, query)
					if testcase.timeout {
						<-r.Context().Done()
						return
					}
					contractResponse(t, w, backend, testcase.rows)
				}))
				defer server.Close()
				client, closeClient := contractClient(t, server.URL, backend)
				defer closeClient()
				ctx := t.Context()
				if testcase.timeout {
					// 本地后端故意阻塞；短时限只验证取消传播，不测试真实网络性能。
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, contractCancelTimeout)
					defer cancel()
				}
				rows, err := client.Search(ctx, query)
				if testcase.timeout {
					if err == nil || ctx.Err() == nil {
						t.Fatalf("取消没有传到后端：%v", err)
					}
					return
				}
				if err != nil || !reflect.DeepEqual(rows, testcase.want) {
					t.Fatalf("结果=%+v 错误=%v，期望=%+v", rows, err, testcase.want)
				}
			})
		}
	}
}

// contractClient 只在装配时区分产品，测试中的调用和断言始终使用同一个接口。
func contractClient(t *testing.T, address, backend string) (source.LogSource, func()) {
	t.Helper()
	if backend == "loki" {
		client, err := loki.New(address)
		if err != nil {
			t.Fatal(err)
		}
		return client, client.Close
	}
	client, err := opensearch.New(opensearch.Config{URL: address, Index: "otel-logs-*", Fields: contractFields()})
	if err != nil {
		t.Fatal(err)
	}
	return client, client.Close
}

// contractRequest 检查请求语义，而不只检查后端回来的数据能否解码。
func contractRequest(t *testing.T, request *http.Request, backend string, query source.LogQuery) {
	t.Helper()
	// 两个后端的查询形状分别断言，上面的行为期望仍共用同一份。
	if backend == "loki" {
		want := "{service=" + strconv.Quote(query.Service) + "}"
		if query.Keyword != "" {
			want += " |= " + strconv.Quote(query.Keyword)
		}
		if query.Level != "" || query.TraceID != "" {
			want += " | json"
		}
		if query.Level != "" {
			want += " | level=" + strconv.Quote(query.Level)
		}
		if query.TraceID != "" {
			want += " | trace_id=" + strconv.Quote(query.TraceID)
		}
		values := request.URL.Query()
		if request.Method != "GET" || request.URL.Path != "/loki/api/v1/query_range" || values.Get("query") != want || values.Get("start") != strconv.FormatInt(query.Start.UnixNano(), 10) || values.Get("end") != strconv.FormatInt(query.End.UnixNano(), 10) || values.Get("limit") != strconv.Itoa(query.Limit) || values.Get("direction") != "backward" {
			t.Errorf("Loki 查询与条件不符：%v", values)
		}
		return
	}
	if request.Method != "POST" || request.URL.Path != "/otel-logs-*/_search" || request.URL.RawQuery != "" {
		t.Errorf("OpenSearch 访问了其他路径或方法：%s %s", request.Method, request.URL.Path)
	}
	var actual any
	if json.NewDecoder(request.Body).Decode(&actual) != nil {
		t.Error("OpenSearch 查询不是合法 JSON")
		return
	}
	fields := contractFields()
	filters := []any{map[string]any{"term": map[string]any{fields.Service: query.Service}}, map[string]any{"range": map[string]any{fields.Timestamp: map[string]any{"gte": query.Start.UTC().Format(time.RFC3339Nano), "lte": query.End.UTC().Format(time.RFC3339Nano), "format": "strict_date_optional_time_nanos"}}}}
	if query.Level != "" {
		filters = append(filters, map[string]any{"term": map[string]any{fields.Level: map[string]any{"value": query.Level, "case_insensitive": true}}})
	}
	if query.TraceID != "" {
		filters = append(filters, map[string]any{"term": map[string]any{fields.TraceID: query.TraceID}})
	}
	condition := map[string]any{"filter": filters}
	if query.Keyword != "" {
		condition["must"] = []any{map[string]any{"match_phrase": map[string]any{fields.Message: query.Keyword}}}
	}
	want := map[string]any{"size": float64(query.Limit), "track_total_hits": false, "query": map[string]any{"bool": condition}, "sort": []any{map[string]any{fields.Timestamp: "desc"}}}
	if !reflect.DeepEqual(actual, want) {
		t.Errorf("OpenSearch 查询翻译错误：%+v", actual)
	}
}

// contractResponse 把同一组日志包装成产品响应，适配器需要自行完成统一排序和截断。
func contractResponse(t *testing.T, writer http.ResponseWriter, backend string, rows []source.LogEntry) {
	t.Helper()
	// 夹具顺序不替适配器预排序，让调用者的倒序契约得到实际验证。
	if backend == "loki" {
		pairs := [][]string{}
		for _, row := range rows {
			pairs = append(pairs, []string{row.Time, row.Line})
		}
		value := map[string]any{"status": "success", "data": map[string]any{"resultType": "streams", "result": []any{map[string]any{"stream": map[string]string{"service": "orders"}, "values": pairs}}}}
		if err := json.NewEncoder(writer).Encode(value); err != nil {
			t.Error(err)
		}
		return
	}
	hits := []any{}
	for _, row := range rows {
		nanos, err := strconv.ParseInt(row.Time, 10, 64)
		if err != nil {
			t.Error(err)
			return
		}
		hits = append(hits, map[string]any{"_source": map[string]any{"resource": map[string]any{"service.name": row.Service}, "body": row.Line, "observedTimestamp": time.Unix(0, nanos).UTC().Format(time.RFC3339Nano)}})
	}
	if err := json.NewEncoder(writer).Encode(map[string]any{"hits": map[string]any{"hits": hits}}); err != nil {
		t.Error(fmt.Errorf("编码模拟日志：%w", err))
	}
}
