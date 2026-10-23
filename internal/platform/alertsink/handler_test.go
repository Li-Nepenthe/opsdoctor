// 本文件：验证告警入口只接受带正确令牌的有限通知。
//
// 测试直接调用 handler，不启动网络服务。日志中只允许出现摘要，不能泄露请求里的秘密。
//
// 主要内容：
//   - TestAlertAuthentication：覆盖缺令牌、错误令牌、成功通知和坏正文。
//
// 阅读建议：每个用例同时检查状态码和输出，不让“返回了 204 但没记录”蒙混过关。

package alertsink

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAlertAuthentication 确认认证在正文解析之前执行，通知正文里的注解也不会原样进入日志。
func TestAlertAuthentication(t *testing.T) {
	const token = "demo-token-used-only-by-unit-test"
	const valid = `{"status":"firing","alerts":[{"status":"firing","labels":{"alertname":"ConsumerLagHigh","service":"order-worker"},"annotations":{"secret":"do-not-log"}}]}`
	for _, testcase := range []struct {
		name          string // 说明要拒绝或接受的场景。
		authorization string // 模拟 Alertmanager 发送的认证头。
		body          string // 正文大小和格式也需要验证。
		status        int    // 只有完整的合法通知返回 204。
	}{
		{"缺令牌", "", valid, 401},                                                         // 不能把公开 webhook 当成任意日志写入口。
		{"错误令牌", "Bearer wrong", valid, 401},                                            // 格式像 Bearer 也不代表认证成功。
		{"正确通知", "Bearer " + token, valid, 204},                                         // 合法通知必须实际输出症状摘要。
		{"坏 JSON", "Bearer " + token, "{", 400},                                         // 解析失败不能伪装成已接受。
		{"尾随对象", "Bearer " + token, valid + " {}", 400},                                 // 防止忽略第二份正文。
		{"超大正文", "Bearer " + token, strings.Repeat(" ", webhookBodyLimit) + valid, 400}, // 限制读入内存的量。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			var output bytes.Buffer
			handler := Handler(token, slog.New(slog.NewJSONHandler(&output, nil)))
			request := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", strings.NewReader(testcase.body))
			request.Header.Set("Authorization", testcase.authorization)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != testcase.status {
				t.Fatalf("状态=%d", response.Code)
			}
			if strings.Contains(output.String(), token) || strings.Contains(output.String(), "do-not-log") {
				t.Fatal("敏感内容进入了日志")
			}
			if testcase.status == 204 && !strings.Contains(output.String(), "ConsumerLagHigh") {
				t.Fatal("通知没有记录")
			}
			if testcase.status != 204 && output.Len() != 0 {
				t.Fatal("无效通知不应留下成功日志")
			}
		})
	}
}
