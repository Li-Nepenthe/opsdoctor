// 本文件：用本地 HTTP 后端验证 OpenSearch 适配器的安全边界和响应解析。
//
// 公共查询行为在上一层的契约表里验证。这里补产品独有的失败情形：字段缺失、
// 部分分片失败、重定向和超大响应。所有账号与响应都是测试夹具。
//
// 主要内容：TestConfigBoundary 验证部署配置；TestResponseBoundary 检查异常响应；
// TestReadOnlyBoundary 证明只读路径和账号不会跟随跳转。

package opensearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source/httpbackend"
)

// testConfig 故意让服务和级别使用 keyword 子字段，读取 _source 时仍要找到原字段。
func testConfig(address string) Config {
	return Config{URL: address, Index: "otel-logs-*", Fields: Fields{Service: "resource.service.name.keyword", Level: "severity.text.keyword", Message: "body", Timestamp: "observedTimestamp", TraceID: "traceId"}}
}

// testQuery 固定一个短窗口，测试不依赖机器当前时间。
func testQuery() source.LogQuery {
	return source.LogQuery{Service: "orders", Start: time.Unix(1, 0), End: time.Unix(2, 0), Limit: 2}
}

// TestConfigBoundary 拒绝能改变路径或扩大索引范围的配置，不等到查询时才失败。
func TestConfigBoundary(t *testing.T) {
	for _, testcase := range []struct {
		name   string        // 要拒绝的错误类型。
		change func(*Config) // 只破坏一个条件，错误原因才明确。
	}{
		{"任意路径", func(c *Config) { c.URL += "/other" }},
		{"URL账号", func(c *Config) { c.URL = "http://reader:fake-password@localhost" }},
		{"全部索引", func(c *Config) { c.Index = "*" }},
		{"多索引", func(c *Config) { c.Index = "logs,private" }},
		{"索引路径", func(c *Config) { c.Index = "logs/../private" }},
		{"字段缺项", func(c *Config) { c.Fields.TraceID = "" }},
		{"字段语法", func(c *Config) { c.Fields.Service = "service[*]" }},
		{"凭据缺项", func(c *Config) { c.Username = "reader" }},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			config := testConfig("http://127.0.0.1:1")
			testcase.change(&config)
			if _, err := New(config); err == nil {
				t.Fatal("错误配置未被拒绝")
			}
		})
	}
}

// TestResponseBoundary 对缺字段返回可解释错误，防止空指针或把部分查询说成完整结果。
func TestResponseBoundary(t *testing.T) {
	for _, testcase := range []struct {
		name, body string // 模拟产品响应，不访问真实索引。
		bad        bool   // 缺消息允许空正文，缺来源或时间则失败。
		line, when string // 成功时需要保留的统一字段。
	}{
		{"嵌套字段", `{"hits":{"hits":[{"_source":{"resource":{"service":{"name":"orders"}},"body":"nested","observedTimestamp":"1970-01-01T00:00:01.123456789Z"}}]}}`, false, "nested", "1123456789"},
		{"完整点键", `{"hits":{"hits":[{"_source":{"resource.service.name":"orders","body":"flat","observedTimestamp":1123456789}}]}}`, false, "flat", "1123456789"},
		{"缺消息", `{"hits":{"hits":[{"_source":{"resource":{"service.name":"orders"},"observedTimestamp":"1970-01-01T00:00:01Z"}}]}}`, false, "", "1000000000"},
		{"结构化消息", `{"hits":{"hits":[{"_source":{"resource":{"service.name":"orders"},"body":{"detail":"x"},"observedTimestamp":"1970-01-01T00:00:01Z"}}]}}`, false, `{"detail":"x"}`, "1000000000"},
		{"别的服务", `{"hits":{"hits":[{"_source":{"resource":{"service.name":"payments"},"observedTimestamp":"1970-01-01T00:00:01Z"}}]}}`, true, "", ""},
		{"缺服务", `{"hits":{"hits":[{"_source":{"body":"x","observedTimestamp":"1970-01-01T00:00:01Z"}}]}}`, true, "", ""},
		{"缺时间", `{"hits":{"hits":[{"_source":{"resource":{"service.name":"orders"}}}]}}`, true, "", ""},
		{"坏时间", `{"hits":{"hits":[{"_source":{"resource":{"service.name":"orders"},"observedTimestamp":"yesterday"}}]}}`, true, "", ""},
		{"部分超时", `{"timed_out":true,"hits":{"hits":[]}}`, true, "", ""},
		{"分片失败", `{"_shards":{"failed":1},"hits":{"hits":[]}}`, true, "", ""},
		{"无效JSON", `{broken`, true, "", ""},
		{"缺命中结构", `{}`, true, "", ""},
		{"多段JSON", `{"hits":{"hits":[]}} {}`, true, "", ""},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(testcase.body)) }))
			defer server.Close()
			client, err := New(testConfig(server.URL))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			rows, err := client.Search(t.Context(), testQuery())
			if (err != nil) != testcase.bad {
				t.Fatalf("错误分类不符：%v", err)
			}
			if !testcase.bad && (len(rows) != 1 || rows[0].Line != testcase.line || rows[0].Time != testcase.when) {
				t.Fatalf("解析结果：%+v", rows)
			}
		})
	}
}

// TestReadOnlyBoundary 既检查唯一的搜索路径，也检查账号不跟随跳转，响应不会无界读入。
func TestReadOnlyBoundary(t *testing.T) {
	// 后端只接受固定索引的搜索，任何跳转后的第二次请求都会使路径检查失败。
	for _, mode := range []string{"正常账号", "拒绝跳转", "过大响应", "后端错误", "请求取消"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/otel-logs-*/_search" {
					t.Errorf("非搜索请求：%s %s", r.Method, r.URL.Path)
				}
				user, password, ok := r.BasicAuth()
				if !ok || user != "reader" || password != "fake-password" {
					t.Error("未使用指定只读账号")
				}
				switch mode {
				case "拒绝跳转":
					http.Redirect(w, r, "/private", http.StatusTemporaryRedirect)
				case "过大响应":
					_, _ = w.Write([]byte(strings.Repeat("x", httpbackend.MaximumBytes+1)))
				case "后端错误":
					http.Error(w, "fake-password should stay private", http.StatusForbidden)
				default:
					_, _ = w.Write([]byte(`{"hits":{"hits":[]}}`))
				}
			}))
			defer server.Close()
			config := testConfig(server.URL)
			config.Username, config.Password = "reader", "fake-password"
			client, err := New(config)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			ctx := t.Context()
			if mode == "请求取消" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			_, err = client.Search(ctx, testQuery())
			if (err != nil) != (mode != "正常账号") {
				t.Fatalf("错误分类：%v", err)
			}
			if err != nil && strings.Contains(err.Error(), "fake-password") {
				t.Fatal("错误回显了后端正文或凭据")
			}
		})
	}
}

// TestKeywordJSONValues 检查特殊字符经过编解码仍是原值，未变成可执行查询语言。
func TestKeywordJSONValues(t *testing.T) {
	// 特殊符号只有在值里保留原样才算通过，不能只证明编码器没有返回错误。
	for _, keyword := range []string{`"quoted"`, `C:\logs\file`, `*:*) OR secret:*`, `x"}],"query_string":{"query":"*"}`} {
		query := testQuery()
		query.Keyword = keyword
		raw, err := json.Marshal(expression(query, testConfig("http://localhost").Fields))
		if err != nil || !json.Valid(raw) {
			t.Fatalf("查询不是合法JSON：%v", err)
		}
		var decoded map[string]any
		if json.Unmarshal(raw, &decoded) != nil {
			t.Fatal("查询无法解码")
		}
		condition := decoded["query"].(map[string]any)["bool"].(map[string]any)
		phrase := condition["must"].([]any)[0].(map[string]any)["match_phrase"].(map[string]any)["body"]
		if phrase != keyword || len(condition) != 2 {
			t.Fatal("关键词改变了查询结构")
		}
	}
}
