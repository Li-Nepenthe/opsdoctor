// 本文件：把每种只读工具交给模型的紧凑 JSON 固定下来，包括超过 4 KiB 的预览。
//
// 假数据源只提供原始观察，摘要和截断仍经过正式注册表。每行是一条完整结果，
// 遇到变化时可以按工具名查到对应的那行。

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"testing/synctest"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
)

// TestToolResultsGolden 同时记录全部菜单和额外一次长日志查询，菜单新增工具也必须有样例。
func TestToolResultsGolden(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		backend := goldenSources{}
		books, err := LoadRunbooks(fstest.MapFS{"metrics.md": {Data: []byte("# 指标\n\n先核对指标窗口。")}})
		if err != nil {
			t.Fatal(err)
		}
		registry := New(Dependencies{
			Metrics: backend, Traces: goldenTraces{}, Logs: goldenLogs{}, MySQL: backend,
			Redis: backend, Profiles: backend, Profile: "订单服务查询库存。",
		}).WithRunbooks(books)
		args := map[string]string{
			"get_trace":          `{"trace_id":"0011223344556677"}`,
			"goroutine_summary":  `{"service":"orders","limit":1}`,
			"kafka_consumer_lag": "{}", "list_firing_alerts": "{}",
			"mysql_explain":    `{"sql":"SELECT id FROM orders"}`,
			"mysql_lock_waits": "{}", "mysql_show_indexes": `{"table":"orders"}`,
			"mysql_status": "{}", "mysql_table_stats": `{"table":"orders"}`, "mysql_top_queries": "{}",
			"query_metrics": `{"query":"up"}`, "recent_changes": "{}", "redis_info": "{}",
			"runbook_search": `{"query":"指标"}`, "search_logs": `{"service":"orders"}`,
			"search_traces": `{"service":"orders"}`, "service_topology": "{}",
		}

		// List 的顺序也是对外约定。记录实际菜单，避免只遍历用例而漏掉新工具。
		var got bytes.Buffer
		for _, def := range registry.List() {
			raw, ok := args[def.Name]
			if !ok {
				t.Fatalf("工具 %s 缺少结果样例", def.Name)
			}
			writeToolResult(t, &got, registry, def.Name, raw, false)
		}
		writeToolResult(t, &got, registry, "search_logs", `{"service":"orders","keyword":"长日志"}`, true)
		want, err := os.ReadFile("testdata/tool_results.golden.jsonl")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.Bytes(), want) {
			offset := 0
			for offset < min(got.Len(), len(want)) && got.Bytes()[offset] == want[offset] {
				offset++
			}
			t.Fatalf("工具结果第 %d 行不同（字节偏移 %d）", bytes.Count(got.Bytes()[:offset], []byte("\n"))+1, offset)
		}
	})
}

// writeToolResult 先核实样例确实走到成功或截断分支，再按模型使用的紧凑格式记录。
func writeToolResult(t *testing.T, out *bytes.Buffer, registry *Registry, name, args string, truncated bool) {
	t.Helper()
	result := registry.Run(t.Context(), name, json.RawMessage(args))
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" || result.Truncated != truncated || result.Bytes != len(raw) || len(raw) > MaximumResultBytes {
		t.Fatalf("%s status=%s truncated=%t bytes=%d/%d：%s", name, result.Status, result.Truncated, result.Bytes, len(raw), result.Summary)
	}
	out.Write(raw)
	out.WriteByte('\n')
}

// goldenSources 给每种查询返回可区分的观察，工具串用数据源或漏掉摘要字段都会改变输出。
type goldenSources struct{}

// Query 让变更查询捕获窗口内的一次重启和版本采样，其他指标同时有极值和末尾非数字样本。
func (goldenSources) Query(_ context.Context, q source.MetricQuery) (source.MetricResult, error) {
	series := source.Series{Labels: map[string]string{"service": "orders"}}
	switch q.Expression {
	case "process_start_time_seconds":
		series.Labels["instance"] = "orders-1"
		series.Points = []source.Point{{Value: strconv.FormatInt(q.Start.Add(time.Minute).Unix(), 10)}}
	case "app_build_info":
		series.Labels["version"] = "v2"
		series.Points = []source.Point{{Time: float64(q.Start.Unix()), Value: "1"}, {Time: float64(q.End.Unix()), Value: "1"}}
	default:
		series.Labels["metric"] = q.Expression
		series.Points = []source.Point{{Value: "4"}, {Value: "1"}, {Value: "NaN"}}
	}
	return source.MetricResult{Series: []source.Series{series}, Limited: true}, nil
}

// Alerts 留下一条含标签和说明的告警，连同触发时间一起交给模型。
func (goldenSources) Alerts(context.Context) ([]source.Alert, error) {
	return []source.Alert{{
		Labels: map[string]string{"alertname": "HighLatency"}, Annotations: map[string]string{"summary": "延迟上升"},
		ActiveAt: time.Unix(123, 0).UTC(), Value: "1",
	}}, nil
}

// metadataRow 为每个元数据查询保留自己的列名和值。
func metadataRow(column, value string) source.Table {
	return source.Table{Columns: []string{column}, Rows: [][]string{{value}}, Limited: true}
}

// Explain 在结果中保留接到的查询，使样例也能区分不同的请求。
func (goldenSources) Explain(_ context.Context, sql string) (source.Table, error) {
	return metadataRow("plan_for", sql), nil
}

// Indexes 与表统计使用同一个表名，但返回不同的元数据列。
func (goldenSources) Indexes(_ context.Context, table string) (source.Table, error) {
	return metadataRow("index_name", table+"_pk"), nil
}

// TableStats 保存表名和估计行数。
func (goldenSources) TableStats(_ context.Context, table string) (source.Table, error) {
	return metadataRow(table+"_estimated_rows", "42"), nil
}

// TopQueries 让默认的返回条数也进入样例。
func (goldenSources) TopQueries(_ context.Context, limit int) (source.Table, error) {
	return metadataRow("requested_limit", strconv.Itoa(limit)), nil
}

// LockWaits 给出一条等待中的线程记录。
func (goldenSources) LockWaits(context.Context) (source.Table, error) {
	return metadataRow("waiting_thread", "17"), nil
}

// Status 给出一个不会和锁等待混淆的状态值。
func (goldenSources) Status(context.Context) (source.Table, error) {
	return metadataRow("Threads_running", "2"), nil
}

// Info 在假响应里带上实际收到的分区名。
func (goldenSources) Info(_ context.Context, section string) (map[string]string, error) {
	return map[string]string{"section": section, "total_connections_received": "12"}, nil
}

// Goroutines 返回两组不同的堆栈，用 limit=1 的查询固定摘要的总数和展示数。
func (goldenSources) Goroutines(_ context.Context, service string) ([]source.StackGroup, error) {
	return []source.StackGroup{{Count: 5, Stack: service + ".wait"}, {Count: 2, Stack: service + ".run"}}, nil
}

// goldenTraces 让搜索摘要和单条调用树使用同一份父子关系。
type goldenTraces struct{}

// Search 复用按 ID 取到的链，两个工具展示的数据可以互相对照。
func (b goldenTraces) Search(ctx context.Context, _ source.TraceQuery) ([]source.Trace, error) {
	trace, err := b.Get(ctx, "0011223344556677")
	return []source.Trace{trace}, err
}

// Get 故意把较短的子步骤排在前面，摘要应按耗时或父子关系重新组织。
func (goldenTraces) Get(_ context.Context, id string) (source.Trace, error) {
	return source.Trace{ID: id, Spans: []source.Span{
		{ID: "child", ParentID: "root", Service: "stock", Operation: "query", Start: 101, Duration: 1200, Tags: map[string]string{"db.query.text": "SELECT id FROM stock"}},
		{ID: "root", Service: "orders", Operation: "create", Start: 100, Duration: 2400},
	}}, nil
}

// goldenLogs 的两种返回量用于对照完整正文和 JSON 预览。
type goldenLogs struct{}

// Search 让截断边界同时遇到中文和需要 JSON 转义的字符。
func (goldenLogs) Search(_ context.Context, q source.LogQuery) ([]source.LogEntry, error) {
	line := "库存响应变慢"
	if q.Keyword == "长日志" {
		line = strings.Repeat("库存\n\"慢\"", 600)
	}
	return []source.LogEntry{{Time: "123456789", Service: q.Service, Line: line}}, nil
}
