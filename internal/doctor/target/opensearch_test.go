// 本文件：验证第二日志后端的配置、装配和系统隔离。
//
// 同一个目录同时放 shop 和第二系统，两套日志后端都通过真实适配器与工具调用。
// 测试另外检查日志后端二选一、字段缺项和只读账号变量，避免配置错误被静默忽略。
//
// 主要内容：TestLogBackendConfig 校验错误配置；TestLogBackendIsolation 走完整查询接线；
// TestOpenSearchCredentialIsolation 检查账号不能跨系统共用。

package target

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// openSearchYAML 使用和实际采集链路相同的嵌套配置形状，地址由测试服务器替换。
func openSearchYAML(address string) string {
	return "opensearch:\n  url: " + address + "\n  index: otel-logs-*\n  fields:\n    service: resource.service.name.keyword\n    level: severity.text.keyword\n    message: body\n    timestamp: observedTimestamp\n    trace_id: traceId\n"
}

// TestLogBackendConfig 在装配前拒绝配置冲突和缺字段，错误能指出需要修复的位置。
func TestLogBackendConfig(t *testing.T) {
	// 使用完整可用配置，每次只破坏一项，报错才有明确含义。
	valid := openSearchYAML("http://127.0.0.1:1")
	for _, testcase := range []struct{ name, extra, want string }{
		{"二选一", "loki: http://127.0.0.1:2\n" + valid, "只能二选一"},
		{"映射缺项", strings.ReplaceAll(valid, "    trace_id: traceId\n", ""), "trace_id"},
		{"字段通配符", strings.ReplaceAll(valid, "service: resource.service.name.keyword", "service: resource.*"), "fields.service"},
		{"无索引前缀", strings.ReplaceAll(valid, "index: otel-logs-*", "index: '*'"), "index"},
		{"凭据不成对", valid + "  username_env: SHOP_LOG_USER\n", "成对"},
		{"凭据无系统前缀", valid + "  username_env: OTHER_LOG_USER\n  password_env: SHOP_LOG_PASSWORD\n", "SHOP_"},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			directory := t.TempDir()
			writeConfig(t, directory, "shop", testcase.extra)
			_, err := Load(directory, testLookup)
			if err == nil || !strings.Contains(err.Error(), testcase.want) {
				t.Fatalf("错误=%v，期望包含%s", err, testcase.want)
			}
		})
	}
}

// TestLogBackendIsolation 使用同一工具参数查询两个后端，且后端各自只收到自己的调用。
func TestLogBackendIsolation(t *testing.T) {
	// 两个后端返回不同文字，既核对内容，也核对实际收到的调用次数。
	var lokiCalls, searchCalls atomic.Int64
	loki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lokiCalls.Add(1)
		if r.Method != "GET" || r.URL.Path != "/loki/api/v1/query_range" {
			t.Error("Loki访问路径错误")
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[{"stream":{"service":"api"},"values":[["1000000000","shop-log"]]}]}}`))
	}))
	defer loki.Close()
	search := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchCalls.Add(1)
		if r.Method != "POST" || r.URL.Path != "/otel-logs-*/_search" {
			t.Error("OpenSearch访问了别的索引或路径")
		}
		_, _ = w.Write([]byte(`{"hits":{"hits":[{"_source":{"resource":{"service.name":"api"},"body":"warehouse-log","observedTimestamp":"1970-01-01T00:00:01Z"}}]}}`))
	}))
	defer search.Close()
	directory := t.TempDir()
	writeConfig(t, directory, "shop", "loki: "+loki.URL+"\n")
	writeConfig(t, directory, "warehouse", openSearchYAML(search.URL))
	catalog, err := Load(directory, testLookup)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	for _, name := range []string{"shop", "warehouse"} {
		system, _ := catalog.Get(name)
		if slices.Contains(system.Unavailable(), "日志") || len(system.Tools().List()) != 2 {
			t.Fatalf("%s日志能力未正确装配：%v", name, system.Unavailable())
		}
		result := system.Tools().Run(t.Context(), "search_logs", json.RawMessage(`{"service":"api","keyword":"log","lookback_seconds":300,"limit":2}`))
		if result.Status != "ok" || !strings.Contains(string(result.Data), name+"-log") {
			t.Fatalf("%s结果=%+v", name, result)
		}
	}
	if lokiCalls.Load() != 1 || searchCalls.Load() != 1 {
		t.Fatal("两个系统的日志来源发生串用")
	}
}

// TestOpenSearchCredentialIsolation 检查变量名不同但地址和账号相同的错误配置。
func TestOpenSearchCredentialIsolation(t *testing.T) {
	// 只改环境变量名字不等于换了账号，身份必须继续按地址和用户名比较。
	directory := t.TempDir()
	for _, name := range []string{"shop", "warehouse"} {
		writeConfig(t, directory, name, openSearchYAML("http://127.0.0.1:1")+fmt.Sprintf("  username_env: %s_LOG_USER\n  password_env: %s_LOG_PASSWORD\n", strings.ToUpper(name), strings.ToUpper(name)))
	}
	lookup := func(name string) (string, bool) {
		if strings.HasSuffix(name, "_LOG_USER") {
			return "reader", true
		}
		if strings.HasSuffix(name, "_LOG_PASSWORD") {
			return "fake-password", true
		}
		return testLookup(name)
	}
	_, err := Load(directory, lookup)
	if err == nil || !strings.Contains(err.Error(), "共用") || strings.Contains(err.Error(), "fake-password") {
		t.Fatalf("账号隔离检查=%v", err)
	}
}

// TestOpenSearchCredentialsWiring 证明账号来自指定环境变量，创建客户端不会探测后端。
func TestOpenSearchCredentialsWiring(t *testing.T) {
	// 装配应保持惰性：等真正调用日志工具时，才发送带账号的搜索。
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		user, password, ok := r.BasicAuth()
		if !ok || user != "reader" || password != "fake-password" {
			t.Error("只读账号未正确装配")
		}
		_, _ = w.Write([]byte(`{"hits":{"hits":[]}}`))
	}))
	defer server.Close()
	directory := t.TempDir()
	writeConfig(t, directory, "shop", openSearchYAML(server.URL)+"  username_env: SHOP_LOG_USER\n  password_env: SHOP_LOG_PASSWORD\n")
	lookup := func(name string) (string, bool) {
		switch name {
		case "SHOP_LOG_USER":
			return "reader", true
		case "SHOP_LOG_PASSWORD":
			return "fake-password", true
		}
		return testLookup(name)
	}
	catalog, err := Load(directory, lookup)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	if calls.Load() != 0 {
		t.Fatal("装配时不应自动查询后端")
	}
	system, _ := catalog.Get("shop")
	if system.RecoveryWindow() != 3*time.Minute {
		t.Fatal("接入日志改变了恢复观察期")
	}
	result := system.Tools().Run(t.Context(), "search_logs", json.RawMessage(`{"service":"api"}`))
	if result.Status != "ok" || calls.Load() != 1 {
		t.Fatalf("账号查询失败：%+v", result)
	}
}
