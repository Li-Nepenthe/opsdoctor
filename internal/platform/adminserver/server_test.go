// 本文件：在内存中验证健康状态和管理接口边界。
//
// TestHealth 给 New 注入成功或失败的探测函数，确认 200/503 与脱敏响应。没有注入注册表时，/metrics 保持 404。
// 先读用例意图，再看 Recorder 如何替代真实网络响应。
//
// 主要内容：
//   - TestHealth：使用 Recorder 直接调用 handler，不启动 TCP 服务，验证状态和探测上下文。
//   - TestObservabilityEndpoints：管理端口导出注入的指标，并能读取 goroutine 剖析。

package adminserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// TestHealth 使用 Recorder 直接调用 handler，不启动 TCP 服务，验证状态和探测上下文。
func TestHealth(t *testing.T) {
	cases := []struct {
		name string // name 说明这个探测状态代表什么。
		err  error  // err 由假依赖返回，避免访问真实数据库。
		want int    // want 是 Compose 识别的 HTTP 状态。
	}{
		{"依赖可用", nil, http.StatusOK},                                      // 只有探测成功才判 healthy。
		{"依赖失败", errors.New("secret-dsn"), http.StatusServiceUnavailable}, // 503 不带敏感错误详情。
	}
	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			server := New(":0", func(ctx context.Context) error {
				if _, exists := ctx.Deadline(); !exists {
					t.Error("健康探测缺少超时")
				}
				return testcase.err
			})
			recorder := httptest.NewRecorder()
			server.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
			if recorder.Code != testcase.want || strings.Contains(recorder.Body.String(), "secret-dsn") {
				t.Fatalf("响应：%d %s", recorder.Code, recorder.Body.String())
			}
			// 这个用例没有传注册表，不能意外导出进程全局的其他指标。
			recorder = httptest.NewRecorder()
			server.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("没有注册表却导出了 metrics：%d", recorder.Code)
			}
		})
	}
}

// TestObservabilityEndpoints 直接调用管理路由，不启动 HTTP 监听或三十秒的 CPU 采样。
func TestObservabilityEndpoints(t *testing.T) {
	registry := prometheus.NewRegistry()
	counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_requests_total", Help: "测试独立注册表。"})
	registry.MustRegister(counter)
	counter.Inc()
	server := New(":0", func(context.Context) error { return nil }, registry)
	for _, testcase := range []struct {
		path     string // 待读取的管理功能。
		contains string // 响应必须包含实际内容，不能只是 200 空页。
	}{
		{"/metrics", "test_requests_total 1"},                    // 只能看到调用方注入的这份指标。
		{"/debug/pprof/goroutine?debug=1", "goroutine profile:"}, // 剖析确实来自正在运行的 Go 进程。
	} {
		response := httptest.NewRecorder()
		server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, testcase.path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), testcase.contains) {
			t.Fatalf("%s 未提供实际数据：%d", testcase.path, response.Code)
		}
	}
}
