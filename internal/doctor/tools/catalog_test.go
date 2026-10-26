// 本文件：验证完整工具菜单的共同边界，以及调用链摘要遇到坏数据时的行为。
//
// 主要内容：
//   - TestEveryToolRejectsDestination：十六个工具都不能接收地址或系统参数。
//   - TestTraceSummaryBoundaries：环路、缺失父节点和重复排序不会破坏原始数据。
// 阅读建议：假接口没有实现后端操作，若校验漏过，测试会得到 internal 而不是预期的参数错误。

package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
)

// unusedSources 的嵌入接口故意为空。错误参数应在碰到任何数据源之前返回。
type unusedSources struct {
	source.MetricsSource
	source.TraceSource
	source.LogSource
	source.MySQLInspector
	source.RedisInspector
	source.ProfileSource
}

// TestEveryToolRejectsDestination 覆盖完整菜单，不能只拿指标工具代表其余十五个工具。
func TestEveryToolRejectsDestination(t *testing.T) {
	unused := &unusedSources{}
	registry := New(Dependencies{Metrics: unused, Traces: &struct{ source.TraceSource }{}, Logs: &struct{ source.LogSource }{}, MySQL: unused, Redis: unused, Profiles: unused})
	if len(registry.List()) != 16 {
		t.Fatal("工具数量改变，请核对完整菜单")
	}
	for _, definition := range registry.List() {
		// 每个字段独立试一次，避免前一个未知字段掩盖后一个被错误接受的字段。
		for _, field := range []string{"url", "host", "target", "system"} {
			t.Run(definition.Name+"/"+field, func(t *testing.T) {
				raw, _ := json.Marshal(map[string]string{field: "outside"})
				result := registry.Run(t.Context(), definition.Name, raw)
				if result.Status != "invalid_argument" {
					t.Fatalf("工具越界：%+v", result)
				}
			})
		}
	}
}

// TestTraceSummaryBoundaries 检查摘要明确标记不完整信息，而不是递归不结束或假造父节点。
func TestTraceSummaryBoundaries(t *testing.T) {
	trace := source.Trace{ID: "demo", Spans: []source.Span{
		{ID: "a", ParentID: "b", Service: "orders", Duration: 10}, // 两节点环路没有根节点。
		{ID: "b", ParentID: "a", Service: "inventory", Duration: 20},
		{ID: "c", ParentID: "missing", Service: "inventory", Tags: map[string]string{"db.query.text": "SELECT * FROM stock WHERE sku_id = ?"}}, // 丢失父节点仍保留 SQL 线索。
	}}
	data, err := json.Marshal(summarizeTree(trace).data)
	if err != nil || !strings.Contains(string(data), "可能存在环路") || !strings.Contains(string(data), "父节点未采集") || !strings.Contains(string(data), "SQL模板") {
		t.Fatalf("树摘要不完整：%s %v", data, err)
	}
	_ = summarizeTraces([]source.Trace{trace})
	if trace.Spans[0].ID != "a" {
		t.Fatal("摘要排序修改了源数据")
	}
}
