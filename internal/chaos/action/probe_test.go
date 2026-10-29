// 本文件：验证安全实验只产生普通 GET，不接受任意主机或执行文字。
//
// 主要内容：TestProbeLog 检查真实请求路径、客户端说明、拒绝跳转和非回环地址。
// 阅读建议：服务器是 httptest 创建的本机夹具，没有商品库。

package action

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestProbeLog 观察实际收到的请求，确保攻击文字只在请求头里，不进入命令或 SQL。
func TestProbeLog(t *testing.T) {
	// 记录服务端实际请求数，才能确认重定向没有偷偷多发一次。
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		if request.Method != "GET" || request.URL.Path != "/products/1" || request.Header.Get("User-Agent") != ProbeText {
			t.Error("没有使用正常商品查询")
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if err := ProbeLog(t.Context(), server.URL); err != nil || calls != 1 {
		t.Fatal(err)
	}
	for _, address := range []string{"http://example.com", server.URL + "/admin", server.URL + "?secret=demo"} {
		if err := ProbeLog(t.Context(), address); err == nil {
			t.Fatal("放行任意地址")
		}
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, server.URL+"/products/1", http.StatusFound)
	}))
	defer redirect.Close()
	if err := ProbeLog(t.Context(), redirect.URL); err == nil || calls != 1 {
		t.Fatal("跟随了跳转")
	}
}
