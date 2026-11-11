// 本文件：用 Jaeger 2.20.0 的真实样例检查查询契约，并补异常边界。
//
// httptest 返回的是已脱敏的本机样例；测试本身不连接运行中的 Jaeger。

package jaeger

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source/httpbackend"
)

// TestRealContract 保留真实三服务结构，防止只用手写的小 JSON 漏掉实际字段关系。
func TestRealContract(t *testing.T) {
	search, err := os.ReadFile("testdata/search.json")
	if err != nil {
		t.Fatal(err)
	}
	detail, err := os.ReadFile("testdata/detail.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct{ Data []wireTrace }
	if err = json.Unmarshal(detail, &fixture); err != nil || len(fixture.Data) != 1 {
		t.Fatalf("夹具损坏：%v", err)
	}
	id := fixture.Data[0].TraceID
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/traces":
			// 起止时间是 Unix 微秒，minDuration 是带单位的字符串，不能混成毫秒数字。
			query := r.URL.Query()
			if query.Get("service") != "order-service" || query.Get("operation") != "POST /orders" || query.Get("start") != "100000000" || query.Get("end") != "200000000" || query.Get("minDuration") != "200ms" || query.Get("limit") != "1" {
				t.Errorf("请求契约错误：%v", query)
			}
			_, _ = w.Write(search)
		case "/api/traces/" + id:
			_, _ = w.Write(detail)
		default:
			t.Errorf("意外路径 %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	traces, err := client.Search(t.Context(), source.TraceQuery{Service: "order-service", Operation: "POST /orders", Start: time.Unix(100, 0), End: time.Unix(200, 0), MinDuration: 200 * time.Millisecond, Limit: 1})
	if err != nil || len(traces) != 1 {
		t.Fatalf("搜索：%v %+v", err, traces)
	}
	trace, err := client.Get(t.Context(), id)
	if err != nil || len(trace.Spans) != 24 || traces[0].ID != trace.ID {
		t.Fatalf("详情：%v %+v", err, trace)
	}
	services := make(map[string]bool)
	parents := 0
	sqlTemplates := 0
	for _, span := range trace.Spans {
		services[span.Service] = true
		if span.ParentID != "" {
			parents++
		}
		if strings.Contains(span.Tags["db.query.text"], "?") {
			sqlTemplates++
		}
		if _, ok := span.Tags["db.connection_string"]; ok {
			t.Fatal("连接属性进入了诊断结果")
		}
	}
	if len(services) != 3 || !services["inventory-service"] || !services["order-worker"] || parents == 0 || sqlTemplates == 0 {
		t.Fatalf("关键证据丢失：services=%v parents=%d sql=%d", services, parents, sqlTemplates)
	}
	if _, err = client.Get(t.Context(), "http://another.test/trace"); err == nil {
		t.Fatal("把 URL 当成 trace ID 接受了")
	}
}

// TestDeploymentPrefix 验证部署前缀只改变固定 API 路径，不改变搜索参数或详情标识。
func TestDeploymentPrefix(t *testing.T) {
	search, err := os.ReadFile("testdata/search.json")
	if err != nil {
		t.Fatal(err)
	}
	detail, err := os.ReadFile("testdata/detail.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct{ Data []wireTrace }
	if err = json.Unmarshal(detail, &fixture); err != nil || len(fixture.Data) != 1 {
		t.Fatal("详情夹具无效", err)
	}
	id := fixture.Data[0].TraceID
	for _, prefix := range []string{"", "/", "/jaeger/ui", "/jaeger/ui/", "/observability/jaeger.v2_~-/ui"} {
		t.Run(prefix, func(t *testing.T) {
			// 使用同一份真实样例，防止只有带前缀的假接口通过而根地址行为发生改变。
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet {
					t.Errorf("出现非只读请求 %s", r.Method)
				}
				switch r.URL.Path {
				case strings.TrimSuffix(prefix, "/") + "/api/traces":
					query := r.URL.Query()
					if query.Get("service") != "order-service" || query.Get("operation") != "POST /orders" || query.Get("start") != "100000000" || query.Get("end") != "200000000" || query.Get("limit") != "1" {
						t.Error("前缀改变了原搜索契约", query)
					}
					_, _ = w.Write(search)
				case strings.TrimSuffix(prefix, "/") + "/api/traces/" + id:
					if r.URL.RawQuery != "" {
						t.Error("详情请求多出查询串")
					}
					_, _ = w.Write(detail)
				default:
					t.Errorf("API 没有使用固定前缀：%s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)
			client, err := New(server.URL + prefix)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.Close)
			traces, err := client.Search(t.Context(), source.TraceQuery{Service: "order-service", Operation: "POST /orders", Start: time.Unix(100, 0), End: time.Unix(200, 0), Limit: 1})
			if err != nil || len(traces) != 1 {
				t.Fatal("前缀搜索失败", traces, err)
			}
			trace, err := client.Get(t.Context(), id)
			if err != nil || trace.ID != traces[0].ID || len(trace.Spans) != 24 || calls.Load() != 2 {
				t.Fatal("前缀详情失败", trace.ID, err, calls.Load())
			}
		})
	}
}

// TestInvalidDeploymentPrefix 不让路径编码、跨目录或跳转绕过固定后端边界。
func TestInvalidDeploymentPrefix(t *testing.T) {
	for _, address := range []string{
		"http://example.test/../private", "http://example.test/jaeger/./ui", "http://example.test/jaeger/../ui",
		"http://example.test//jaeger/ui", "http://example.test/jaeger//ui", "http://example.test/jaeger//",
		"http://example.test/jaeger/%2e%2e/ui", "http://example.test/jaeger/%2Fui", "http://example.test/jaeger/%61",
		"http://example.test/jaeger%20ui", "http://example.test/jaeger\\ui", "http://example.test/jaeger/ui;private",
		"http://example.test/jaeger/ui?next=elsewhere", "http://example.test/jaeger/ui?", "http://example.test/jaeger/ui#fragment", "http://example.test/jaeger/ui#",
		"http://user:secret@example.test/jaeger/ui", "file:///jaeger/ui", "//example.test/jaeger/ui",
	} {
		if _, err := New(address); err == nil {
			t.Errorf("接受了危险部署地址：%q", address)
		}
	}
	// Jaeger 的特例不能放宽共享客户端，更不能让部署前缀后的跳转访问另一个地址。
	if _, err := httpbackend.New("http://example.test/jaeger/ui"); err == nil {
		t.Fatal("公共 HTTP 根地址限制被放宽")
	}
	var visits atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { visits.Add(1); w.WriteHeader(http.StatusOK) }))
	t.Cleanup(destination.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusFound)
	}))
	t.Cleanup(server.Close)
	client, err := New(server.URL + "/jaeger/ui")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	if _, err = client.Search(t.Context(), source.TraceQuery{Service: "ad", Start: time.Unix(100, 0), End: time.Unix(200, 0), Limit: 1}); err == nil || visits.Load() != 0 {
		t.Fatal("带前缀的查询跟随了重定向", err, visits.Load())
	}
}

// TestMalformedAndEmpty 检查真实成功样例覆盖不到的布尔属性、空结果和错误路径。
func TestMalformedAndEmpty(t *testing.T) {
	for _, testcase := range []struct {
		name string // 用例意图。
		body string // 只包含该边界需要的最小响应。
		bad  bool   // 应返回错误还是正常结果。
	}{
		{"空搜索", `{"data":[],"errors":null}`, false},                                                                   // 没找到请求不是后端故障。
		{"后端错误", `{"data":[],"errors":[{"msg":"internal"}]}`, true},                                                   // 不能把错误伪装成空结果。
		{"缺少服务", `{"data":[{"traceID":"abc","spans":[{"traceID":"abc","spanID":"a","processID":"missing"}]}]}`, true}, // 无法映射服务时明确失败。
		{"布尔与跨调用链引用", `{"data":[{"traceID":"abc","processes":{"p":{"serviceName":"inventory"}},"spans":[{"traceID":"abc","spanID":"a","processID":"p","references":[{"refType":"CHILD_OF","traceID":"other","spanID":"b"}],"tags":[{"key":"error","type":"bool","value":true},{"key":"rpc.grpc.status_code","type":"int64","value":4}]}]}]}`, false}, // 布尔和数字保留，但不能把其他 trace 当父节点。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(testcase.body)) }))
			t.Cleanup(server.Close)
			client, err := New(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.Close)
			result, err := client.Search(t.Context(), source.TraceQuery{Service: "inventory", Start: time.Unix(1, 0), End: time.Unix(2, 0), Limit: 1})
			if (err != nil) != testcase.bad {
				t.Fatalf("错误分类：%v", err)
			}
			if testcase.name == "布尔与跨调用链引用" && (len(result) != 1 || result[0].Spans[0].ParentID != "" || result[0].Spans[0].Tags["error"] != "true" || result[0].Spans[0].Tags["rpc.grpc.status_code"] != "4") {
				t.Fatalf("属性或引用丢失：%+v", result)
			}
		})
	}
}
