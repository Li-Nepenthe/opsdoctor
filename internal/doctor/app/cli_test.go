// 本文件：验证命令行始终把工具参数交给同一个医生接口。
//
// 主要内容：
//   - TestCLIUsesServer：错误参数也原样送给服务器，再输出服务器的校验结果。
//   - TestCLIRejectsRedirect：登录令牌不能跟着重定向发给另一个地址。
//   - TestCLICommand：未知选项和缺少系统名在发请求前拒绝。
// 阅读建议：这里用本地假 HTTP 服务记录请求，没有数据库或 Docker 依赖。

package app

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestCLIUsesServer 验证 CLI 不会自行吞掉或接受非法参数，最终结果由注册表所在的接口决定。
func TestCLIUsesServer(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/api/v1/targets/shop/tools/mysql_status/run" || r.Method != "POST" || string(body) != `{"host":"elsewhere"}` || r.Header.Get("Authorization") != "Bearer demo-token" {
			t.Errorf("CLI 请求未遵守约定")
		}
		_, _ = io.WriteString(w, `{"data":{"tool":"mysql_status","status":"invalid_argument","summary":"不接受主机参数","data":null}}`)
	}))
	defer server.Close()
	reader := config.NewReader(func(key string) (string, bool) {
		if key == "DOCTOR_URL" {
			return server.URL, true
		}
		if key == "DOCTOR_TOKEN" {
			return "demo-token", true
		}
		return "", false
	})
	var output bytes.Buffer
	err := RunCLI(t.Context(), []string{"tool", "run", "--target", "shop", "mysql_status", "--args", `{"host":"elsewhere"}`}, reader, &output)
	// 错误仍须打印，脚本还要得到失败退出状态，两者缺一都不好排查。
	if err == nil || calls != 1 || !strings.Contains(output.String(), "invalid_argument") {
		t.Fatalf("calls=%d output=%s err=%v", calls, output.String(), err)
	}
}

// TestCLIRejectsRedirect 让第二个服务记录请求数，证明拒绝发生在令牌被转发之前。
func TestCLIRejectsRedirect(t *testing.T) {
	calls := 0
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer server.Close()
	_, err := request(t.Context(), server.URL, "demo-token", "GET", "/api/v1/targets", "")
	if err == nil || calls != 0 {
		t.Fatalf("重定向越界：calls=%d err=%v", calls, err)
	}
}

// TestCLICommand 只测入口语法；工具字段的校验留给服务端注册表。
func TestCLICommand(t *testing.T) {
	for _, args := range [][]string{
		{"target", "list", "--url", "http://other"}, // 不能在命令里临时替换地址。
		{"tool", "list"}, // 必须明确选择系统。
		{"tool", "run", "--target", "../shop", "mysql_status"},                   // 系统名不能改变 URL 路径。
		{"tool", "run", "--target", "shop", "--host", "other"},                   // 不支持后端主机选项。
		{"tool", "run", "--target", "shop", "mysql_status", "--target", "other"}, // 重复选项不能悄悄覆盖。
	} {
		if _, err := parseCommand(args); err == nil {
			t.Fatalf("接受了非法命令 %v", args)
		}
	}
}
