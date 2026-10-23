// 本文件：在内存中验证健康状态和管理接口边界。
//
// TestHealth 给 New 注入成功或失败的探测函数，确认 200/503 与脱敏响应。还检查尚未实现的 /metrics 是 404。
// 先读用例意图，再看 Recorder 如何替代真实网络响应。
//
// 主要内容：
//   - TestHealth：使用 Recorder 直接调用 handler，不启动 TCP 服务，验证状态和探测上下文。

package adminserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
			// 本阶段只有健康接口，未知管理功能必须返回 404，不能宣称指标已经接通。
			recorder = httptest.NewRecorder()
			server.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("未实现的 metrics：%d", recorder.Code)
			}
		})
	}
}
