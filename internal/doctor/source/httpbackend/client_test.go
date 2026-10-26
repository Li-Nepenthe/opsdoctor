// 本文件：验证监控 HTTP 请求不会逃到其他地址，也不会无限读取。
//
// 每个测试都使用本机 httptest 服务，不依赖实际监控栈。
//
// 主要内容：
//   - TestHTTPBoundary：坏配置、重定向、超大正文和取消都应被拒绝。
//
// 阅读建议：先看两个服务器的计数，确认重定向目的地真的没有收到请求。

package httpbackend

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestHTTPBoundary 既检查返回错误，也检查不能发生的外部访问确实没有发生。
func TestHTTPBoundary(t *testing.T) {
	for _, address := range []string{
		"file:///etc/passwd",                            // 非 HTTP 协议不能混成监控服务。
		"http://user:secret@example.test",               // 凭据不放在可被错误打印的 URL 中。
		"http://example.test/private",                   // 配置只提供根地址，路径由适配器决定。
		"http://example.test?url=http://elsewhere.test", // 不允许把可变查询地址藏进配置。
	} {
		if _, err := New(address); err == nil {
			t.Errorf("未拒绝坏地址类型")
		}
	}
	var visits atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { visits.Add(1); w.WriteHeader(200) }))
	t.Cleanup(destination.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, destination.URL, http.StatusFound) // 即使是合法后端给出的跳转，也不能跟随。
		case "/large":
			_, _ = w.Write([]byte(strings.Repeat("x", MaximumBytes+1))) // 多一个字节也必须拒绝。
		case "/wait":
			<-r.Context().Done() // 等调用者取消，验证信号能传给实际 HTTP 请求。
		default:
			_, _ = w.Write([]byte(`{"value":1}`))
		}
	}))
	t.Cleanup(server.Close)
	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	for _, path := range []string{"/redirect", "/large", "//another.test"} {
		if _, err := client.Bytes(t.Context(), path, nil); err == nil {
			t.Errorf("路径 %s 未被拒绝", path)
		}
	}
	if visits.Load() != 0 {
		t.Fatal("跟随了重定向")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.Bytes(ctx, "/wait", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("没有保留取消分类：%v", err)
	}
	var result struct{ Value int }
	if err := client.Get(t.Context(), "/ok", nil, &result); err != nil || result.Value != 1 {
		t.Fatalf("合法响应失败：%v %+v", err, result)
	}
}
