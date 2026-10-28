// 本文件：在内存 HTTP 请求里检查登录、系统令牌和工具入口。
//
// 仓储使用可计数的假实现。重点是证明未经授权的请求根本到不了保存或工具调用，
// 数据库如何处理重复推送则由 store 的真实 MySQL 集成测试负责。
//
// 主要内容：
//   - TestTargetAuthentication：A 的令牌不能向 B 推送，也不能当作登录令牌使用。
//   - TestLoginAndToolRoute：检查登录、同一注册表的参数拒绝和工具执行。
//   - TestNotificationValidation：通知格式错误时不调用仓储。
//   - setupAPI、memoryStore：构造两套虚构系统，不连接任何真实数据源。
//
// 阅读建议：每次拒绝之后都检查仓储计数，不能只看响应状态码。

package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/alerts"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/auth"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/target"
)

// memoryStore 只记录经过 HTTP 校验的调用，不在这里模拟数据库的唯一约束。
type memoryStore struct {
	mu       sync.Mutex // 保护调用记录，允许竞态检查覆盖并发请求。
	received []string   // 每次保存请求真正绑定的系统名。
	hash     string     // 测试账号的 bcrypt 摘要。
}

// ReceiveAlerts 记录身份，便于发现错误的路径或令牌被传入保存层。
func (memory *memoryStore) ReceiveAlerts(_ context.Context, target string, _ alerts.Notification) (alerts.Receipt, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	memory.received = append(memory.received, target)
	return alerts.Receipt{DiagnosisID: strings.Repeat("a", 32), Created: true}, nil
}

// UserByEmail 只有一个虚构用户，未知邮箱与真实仓储一样返回 ErrNoRows。
func (memory *memoryStore) UserByEmail(_ context.Context, email string) (store.User, error) {
	if email != "learner@example.invalid" {
		return store.User{}, sql.ErrNoRows
	}
	return store.User{ID: "test-user", Email: email, PasswordHash: memory.hash, Status: "active"}, nil
}

// List 返回空列表；这个测试不把内存数据假装成真实数据库分页验收。
func (*memoryStore) List(context.Context, string, string, int, int) ([]store.Diagnosis, error) {
	return []store.Diagnosis{}, nil
}

// Detail 用未找到覆盖 HTTP 的 404 映射。
func (*memoryStore) Detail(context.Context, string) (store.Detail, error) {
	return store.Detail{}, sql.ErrNoRows
}

// DiagnosisAlerts 与默认详情一样返回未找到，专项测试再提供告警记录。
func (*memoryStore) DiagnosisAlerts(context.Context, string) (store.AlertHistory, error) {
	return store.AlertHistory{}, sql.ErrNoRows
}

// FaultState 的默认系统没有故障，避免身份测试依赖后台任务状态。
func (*memoryStore) FaultState(context.Context, string) (store.FaultState, error) {
	return store.FaultState{}, nil
}

// Manual 在身份测试里只返回固定编号，具体状态转换另由仓储测试验证。
func (*memoryStore) Manual(context.Context, string, string) (string, error) {
	return strings.Repeat("a", 32), nil
}

// StepsAfter 默认无事件，SSE 专项测试再提供有顺序的步骤。
func (*memoryStore) StepsAfter(context.Context, string, int) ([]agent.Step, error) {
	return []agent.Step{}, nil
}

// setupAPI 的两个系统只有架构工具，任何请求都不需要访问外部后端。
func setupAPI(t *testing.T) (http.Handler, *memoryStore, string) {
	t.Helper()
	return setupWithOptions(t, nil)
}

// setupWithOptions 让 SSE 测试只替换仓储和事件中心，身份校验仍走真实路由。
func setupWithOptions(t *testing.T, modify func(*Options)) (http.Handler, *memoryStore, string) {
	// 两个系统各有独立的令牌；测试还要确认拒绝的通知没有调用存储。
	t.Helper()
	directory := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		config := "name: " + name + "\nprofile: " + name + ".profile.md\nalert_token_env: " + strings.ToUpper(name) + "_ALERT_TOKEN\nalert_recovery_window: 1m\n"
		for file, body := range map[string]string{name + ".yaml": config, name + ".profile.md": "# " + name + "\n一个虚构服务。\n<!-- COMPONENTS_START -->\n| `unknown` | 尚未定位 |\n| `" + name + "` | HTTP 服务 |\n<!-- COMPONENTS_END -->\n"} {
			if err := os.WriteFile(filepath.Join(directory, file), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	catalog, err := target.Load(directory, func(name string) (string, bool) {
		if name == "ALPHA_ALERT_TOKEN" {
			return strings.Repeat("a", 32), true
		}
		return strings.Repeat("b", 32), true
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = catalog.Close() })
	tokens, err := auth.NewTokens(strings.Repeat("j", 32), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	loginToken, err := tokens.Issue("test-user")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("demo-password-only")
	if err != nil {
		t.Fatal(err)
	}
	memory := &memoryStore{hash: hash}
	options := Options{Targets: catalog, Store: memory, Tokens: tokens, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Ready: func(context.Context) error { return nil }, Metrics: http.NotFoundHandler(), DummyPasswordHash: hash}
	if modify != nil {
		modify(&options)
	}
	router := New(options)
	return router, memory, loginToken
}

// apiCall 只把测试令牌放在请求头，不在失败信息中打印请求或凭据。
func apiCall(router http.Handler, method, path, token string, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

// notificationBody 与 Alertmanager 的真实字段一致，额外展示字段允许被忽略。
func notificationBody() []byte {
	return []byte(`{"status":"firing","groupKey":"example","groupLabels":{"alertname":"TestOnly"},"externalURL":"http://example.invalid","alerts":[{"status":"firing","fingerprint":"0123456789abcdef","labels":{"alertname":"TestOnly"},"startsAt":"2026-10-26T00:00:00Z"}]}`)
}

// TestTargetAuthentication 把路径切换和令牌类型混用都当作未经授权处理。
func TestTargetAuthentication(t *testing.T) {
	router, memory, login := setupAPI(t)
	for _, testcase := range []struct{ path, token string }{
		{"/api/v1/targets/beta/alerts", strings.Repeat("a", 32)},    // A 令牌不能推 B。
		{"/api/v1/targets/alpha/alerts", login},                     // 人的 JWT 不能冒充告警来源。
		{"/api/v1/targets/missing/alerts", strings.Repeat("a", 32)}, // 不存在的系统不能退回 A。
		{"/api/v1/targets/alpha/alerts", ""},                        // 没有身份不能读正文后再保存。
	} {
		response := apiCall(router, "POST", testcase.path, testcase.token, notificationBody())
		if response.Code != 401 || len(memory.received) != 0 {
			t.Fatal("未经授权的通知到达仓储")
		}
	}
	response := apiCall(router, "POST", "/api/v1/targets/alpha/alerts", strings.Repeat("a", 32), notificationBody())
	if response.Code != 202 || len(memory.received) != 1 || memory.received[0] != "alpha" {
		t.Fatal("合法通知没有绑定 alpha")
	}
	response = apiCall(router, "GET", "/api/v1/targets", strings.Repeat("a", 32), nil)
	if response.Code != 401 {
		t.Fatal("告警令牌被用作登录令牌")
	}
}

// TestLoginAndToolRoute 从真实登录响应取令牌，再通过 HTTP 进入统一工具校验。
func TestLoginAndToolRoute(t *testing.T) {
	router, _, _ := setupAPI(t)
	wrong := apiCall(router, "POST", "/api/v1/auth/login", "", []byte(`{"email":"learner@example.invalid","password":"wrong-demo-password"}`))
	if wrong.Code != 401 {
		t.Fatal("错误密码被接受")
	}
	response := apiCall(router, "POST", "/api/v1/auth/login", "", []byte(`{"email":"learner@example.invalid","password":"demo-password-only"}`))
	var login struct {
		Data struct {
			Token string `json:"access_token"`
		} `json:"data"`
	}
	if json.Unmarshal(response.Body.Bytes(), &login) != nil || response.Code != 200 || login.Data.Token == "" {
		t.Fatal("登录失败")
	}
	for _, body := range []string{`{"url":"http://example.invalid"}`, `{"host":"other"}`, `{"system":"beta"}`, `{"target":"beta"}`, `{"nested":{"url":"http://example.invalid"}}`} {
		// 即使只有架构工具，也不能接受“换地址”字段；所有入口遵守同一规则。
		result := apiCall(router, "POST", "/api/v1/targets/alpha/tools/service_topology/run", login.Data.Token, []byte(body))
		if result.Code != 200 || !strings.Contains(result.Body.String(), `"status":"invalid_argument"`) {
			t.Fatal("HTTP 绕过工具校验")
		}
	}
	result := apiCall(router, "POST", "/api/v1/targets/alpha/tools/service_topology/run", login.Data.Token, []byte(`{}`))
	if result.Code != 200 || !strings.Contains(result.Body.String(), "alpha") || strings.Contains(result.Body.String(), "一个 beta") {
		t.Fatal("架构工具调用失败")
	}
	missing := apiCall(router, "POST", "/api/v1/targets/missing/tools/service_topology/run", login.Data.Token, []byte(`{}`))
	if missing.Code != 404 {
		t.Fatal("未知系统未被拒绝")
	}
}

// TestNotificationValidation 证明无效通知不会留下半条任务记录。
func TestNotificationValidation(t *testing.T) {
	router, memory, _ := setupAPI(t)
	for _, body := range [][]byte{
		[]byte(`{}`), // 没有分组和告警。
		append(notificationBody(), []byte(`{}`)...),                                                     // 正文多出第二个对象。
		[]byte(strings.ReplaceAll(string(notificationBody()), "0123456789abcdef", "bad")),               // 指纹不是标准长度。
		[]byte(strings.ReplaceAll(string(notificationBody()), `"groupKey":"example"`, `"groupKey":""`)), // 不能用空分组把所有通知合并。
	} {
		response := apiCall(router, "POST", "/api/v1/targets/alpha/alerts", strings.Repeat("a", 32), body)
		if response.Code != 400 || len(memory.received) != 0 {
			t.Fatal("无效通知调用了仓储")
		}
	}
}
