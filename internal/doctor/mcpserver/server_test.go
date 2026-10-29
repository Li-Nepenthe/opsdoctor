// 本文件：用官方 MCP 客户端验证身份、工具边界和审计确实贯通。
//
// 测试只连接 httptest 的本机服务器。真实查询仍由原注册表处理，工具里的架构文字
// 分别写 A、B，能看出有没有串到另一个系统；不需要真实模型或业务数据库。
//
// 主要内容：TestClientBoundary 检查同一注册表与审计；TestTokenIsolation 检查跨系统令牌。
// 阅读建议：客户端只在 HTTP 请求头加令牌，地址和工具参数里都没有秘密。

package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// memoryAudit 记录 SDK 后台请求的审计，互斥锁避免测试读写碰撞。
type memoryAudit struct {
	mu       sync.Mutex // 客户端请求与测试断言在不同 goroutine。
	entries  []Entry    // 每次调用都必须先登记。
	statuses []string   // 与 entries 逐项对应。
}

// BeginMCP 返回编号而非直接执行工具，使测试可以核对每条调用是否留下起点。
func (audit *memoryAudit) BeginMCP(_ context.Context, entry Entry) (string, error) {
	audit.mu.Lock()
	defer audit.mu.Unlock()
	audit.entries = append(audit.entries, entry)
	return fmt.Sprint(len(audit.entries)), nil
}

// EndMCP 只记有限状态，测试不需要模仿数据库事务；真实事务由 store 集成测试负责。
func (audit *memoryAudit) EndMCP(_ context.Context, _ string, status string) error {
	audit.mu.Lock()
	defer audit.mu.Unlock()
	audit.statuses = append(audit.statuses, status)
	return nil
}

// bearerTransport 只给这个测试客户端附上固定令牌，不设置包级默认 HTTP 认证。
type bearerTransport struct{ token string }

// RoundTrip 克隆请求再加头，避免修改官方 SDK 仍可能使用的请求对象。
func (transport bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())
	cloned.Header.Set("Authorization", "Bearer "+transport.token)
	return http.DefaultTransport.RoundTrip(cloned)
}

// connect 使用真正的官方客户端协议，不通过直接调用 handler 跳过初始化和序列化。
func connect(t *testing.T, url, token string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "opsdoctor-test", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: url, HTTPClient: &http.Client{Transport: bearerTransport{token}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// TestClientBoundary 合法、非法参数与不存在的写工具都要留审计，列表只能看见只读工具。
func TestClientBoundary(t *testing.T) {
	token := strings.Repeat("a", 32)
	audit := &memoryAudit{}
	handler, err := New("system-a", token, tools.New(tools.Dependencies{Profile: "system-a 架构"}), audit, llm.NewRedactor())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client := connect(t, server.URL, token)
	menu, err := client.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range menu.Tools {
		if item.Name == "create_index" || item.Name == "warm_cache" || item.Name == "set_db_pool_size" {
			t.Fatal("MCP 暴露了写工具")
		}
	}
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "service_topology", Arguments: map[string]any{}})
	if err != nil || result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "system-a") {
		t.Fatalf("结果=%+v %v", result, err)
	}
	// 主机和系统参数不能绕过原有 Schema；错误仍返回工具结果，供客户端修正。
	result, err = client.CallTool(t.Context(), &mcp.CallToolParams{Name: "service_topology", Arguments: map[string]any{"target": "system-b", "host": "other.invalid", "password": "demo-password"}})
	if err != nil || !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "invalid_argument") {
		t.Fatal("MCP 绕过参数校验")
	}
	if _, err = client.CallTool(t.Context(), &mcp.CallToolParams{Name: "create_index", Arguments: map[string]any{}}); err == nil {
		t.Fatal("执行了不存在的写工具")
	}
	audit.mu.Lock()
	defer audit.mu.Unlock()
	if len(audit.entries) != 3 || len(audit.statuses) != 3 || audit.statuses[0] != "ok" || audit.statuses[1] != "tool_error" || audit.statuses[2] != "protocol_error" {
		t.Fatalf("审计=%+v %+v", audit.entries, audit.statuses)
	}
	for _, entry := range audit.entries {
		if entry.Target != "system-a" || strings.Contains(entry.Arguments, "demo-password") {
			t.Fatal("审计串系统或未脱敏")
		}
	}
}

// TestTokenIsolation A 的令牌不能访问 B；就算地址正确，也要先被 HTTP 身份边界挡住。
func TestTokenIsolation(t *testing.T) {
	handler, err := New("system-b", strings.Repeat("b", 32), tools.New(tools.Dependencies{Profile: "system-b 架构"}), &memoryAudit{}, llm.NewRedactor())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	for _, token := range []string{"", strings.Repeat("a", 32), "login-jwt-demo"} {
		// 分别模拟匿名、另一系统的长期令牌、以及网页短期登录令牌。
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL, strings.NewReader(`{}`))
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != 401 {
			t.Fatal("错误身份被放行")
		}
	}
	client := connect(t, server.URL, strings.Repeat("b", 32))
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "service_topology", Arguments: map[string]any{}})
	if err != nil || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "system-b") {
		t.Fatal("B 的入口没有绑定 B 注册表")
	}
}
