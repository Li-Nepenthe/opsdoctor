// 本文件：验证只查已配置的服务，并把相同函数栈的数量合并。
//
// 夹具保留 Go debug=1 的文本形状，不使用实际内存地址判断两个堆栈是否相同。
//
// 主要内容：
//   - TestGoroutines：固定路径、服务白名单和地址归一化。
//
// 阅读建议：两组内存地址不同，但函数和文件相同，结果应合并成五个任务。

package pprof

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestGoroutines 确认未知服务在联网前被拒绝，而不只是最后返回 404。
func TestGoroutines(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/debug/pprof/goroutine" || r.URL.Query().Get("debug") != "1" {
			t.Errorf("错误路径：%s", r.URL.Path)
		}
		_, _ = w.Write([]byte("goroutine profile: total 5\n2 @ 0xaaa\n# 0xaaa example.wait+0x1 /app/wait.go:10\n\n3 @ 0xbbb\n# 0xbbb example.wait+0x2 /app/wait.go:10\n"))
	}))
	t.Cleanup(server.Close)
	addresses := map[string]string{"orders": server.URL}
	client, err := New(addresses)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	addresses["another"] = "http://example.test" // 原 map 后来改变，也不能扩充已有客户端的访问范围。
	for _, service := range []string{"another", "http://example.test"} {
		if _, err := client.Goroutines(t.Context(), service); err == nil {
			t.Fatal("未知服务被放行")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("未知服务触发了请求")
	}
	groups, err := client.Goroutines(t.Context(), "orders")
	if err != nil || len(groups) != 1 || groups[0].Count != 5 || groups[0].Stack != "example.wait /app/wait.go:10" {
		t.Fatalf("分组：%v %+v", err, groups)
	}
	if _, err = parse([]byte("not a profile")); err == nil {
		t.Fatal("坏文本被当成空成功")
	}
}
