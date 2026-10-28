// 本文件：从浏览器 Cookie 路径验证登录和 CSRF 边界。
//
// 只检查 JWT 签名不够：Cookie 会被浏览器自动带上。测试分别覆盖正常页面、外站
// 请求和命令行 Bearer，保证增加 SSE 登录方式时没有取消原来的身份边界。
//
// 主要内容：TestCookieSession 检查 Cookie 属性、修改校验和 CLI；TestCrossSiteLogin 拒绝外站登录。
// 阅读建议：测试凭据都是明显的演示字符串，失败信息不打印完整请求头。

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCookieSession 先真实登录，再用返回 Cookie 请求，避免绕过签发流程拼一个假会话。
func TestCookieSession(t *testing.T) {
	router, _, bearer := setupAPI(t)
	login := apiCall(router, "POST", "/api/v1/auth/login", "", []byte(`{"email":"learner@example.invalid","password":"demo-password-only"}`))
	cookies := login.Result().Cookies()
	if login.Code != 200 || len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Path != "/" {
		t.Fatal("登录 Cookie 属性不正确")
	}
	var body struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		} `json:"data"`
	}
	if json.Unmarshal(login.Body.Bytes(), &body) != nil || body.Data.CSRF == "" {
		t.Fatal("未返回 CSRF 值")
	}
	for _, testcase := range []struct {
		name, csrf, origin string
		status             int
	}{
		{"缺 CSRF", "", "", 403},                                 // Cookie 自动附带，单凭 Cookie 不能执行修改请求。
		{"别的会话", "wrong-demo-value", "", 403},                   // 值必须与当前会话绑定。
		{"外站", body.Data.CSRF, "https://attacker.invalid", 403}, // 即使知道值也拒绝跨站来源。
		{"当前页面", body.Data.CSRF, "http://example.com", 200},     // 同源 Cookie 与派生值一起通过。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/api/v1/targets/alpha/tools/service_topology/run", bytes.NewBufferString(`{}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-CSRF-Token", testcase.csrf)
			request.Header.Set("Origin", testcase.origin)
			request.AddCookie(cookies[0])
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != testcase.status {
				t.Fatalf("状态=%d", response.Code)
			}
		})
	}
	// CLI 主动发送 Authorization，不依赖 Cookie，仍可直接调用同一个工具入口。
	if result := apiCall(router, "POST", "/api/v1/targets/alpha/tools/service_topology/run", bearer, []byte(`{}`)); result.Code != 200 {
		t.Fatal("Bearer 调用被 CSRF 错误拦截")
	}
	request := httptest.NewRequest("GET", "/api/v1/auth/session", nil)
	request.AddCookie(cookies[0])
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("会话刷新或缓存边界错误")
	}
	if json.Unmarshal(response.Body.Bytes(), &body) != nil || !validCSRF(cookies[0].Value, body.Data.CSRF) {
		t.Fatal("刷新后的 CSRF 与会话不匹配")
	}
}

// TestCrossSiteLogin 拒绝攻击者替用户登录另一个账号，同时页面启用禁止内联脚本的策略。
func TestCrossSiteLogin(t *testing.T) {
	router, _, _ := setupAPI(t)
	request := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(`{"email":"learner@example.invalid","password":"demo-password-only"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://attacker.invalid")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 403 || len(response.Result().Cookies()) != 0 {
		t.Fatal("跨站登录未拒绝")
	}
	page := apiCall(router, "GET", "/", "", nil)
	if page.Code != 200 || page.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("页面安全策略缺失")
	}
}
