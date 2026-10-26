// 本文件：用 Jaeger 2.20.0 的真实样例检查查询契约，并补异常边界。
//
// httptest 返回的是已脱敏的本机样例；测试本身不连接运行中的 Jaeger。
//
// 主要内容：
//   - TestRealContract：检查请求路径、时间单位、服务映射和父子关系。
//   - TestMalformedAndEmpty：空结果、坏属性、跨 trace 引用和后端错误。
//
// 阅读建议：先对照 testdata/README.md，再看断言为什么不只检查 HTTP 200。

package jaeger

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
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
