package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/logging"
	"github.com/gin-gonic/gin"
)

// testRouter 组装纯内存路由，不监听端口；handler 注入每个用例要验证的行为。
func testRouter(output *bytes.Buffer, handler gin.HandlerFunc) *gin.Engine {
	router := gin.New()
	logger := logging.New(output, "test-http", slog.LevelInfo)
	// 顺序很重要：恢复 panic 和访问日志都需要最先写入的 request_id。
	router.Use(RequestIDs(logger), Recovery(logger), BodyLimit(DefaultBodyLimit))
	router.POST("/test", handler)
	return router
}

// TestRequestIDAndRecovery 验证合法标识保留、非法标识替换，以及 panic 不泄露原始内容。
func TestRequestIDAndRecovery(t *testing.T) {
	for _, id := range []string{"request-ok", "", "unsafe/value"} {
		t.Run(id, func(t *testing.T) {
			var logs bytes.Buffer
			router := testRouter(&logs, func(*gin.Context) { panic("private-password-value") })
			request := httptest.NewRequest(http.MethodPost, "/test?password=private-query", nil)
			request.Header.Set("X-Request-ID", id)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			// 比较响应头、响应体和日志，三个位置必须使用同一个标识。
			actual := response.Header().Get("X-Request-ID")
			if !validRequestID(actual) || (id == "request-ok" && actual != id) {
				t.Fatalf("请求标识不合法：%q", actual)
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusInternalServerError || body["code"] != "INTERNAL" || body["request_id"] != actual {
				t.Fatalf("错误响应不符合契约：%d %v", response.Code, body)
			}
			if strings.Contains(logs.String()+response.Body.String(), "private-") || !strings.Contains(logs.String(), actual) {
				t.Fatal("日志泄露了输入，或缺少请求标识")
			}
		})
	}
}

// TestJSONBodyLimit 检查未知字段、多个 JSON、已知长度和流式超限，防止只检查 Content-Length。
func TestJSONBodyLimit(t *testing.T) {
	for _, test := range []struct {
		name   string // name 描述需要防御的输入场景。
		body   string // body 只使用演示请求，不包含真实凭据。
		stream bool   // stream 为 true 时模拟没有 Content-Length 的分块请求。
		status int    // status 是期待的 HTTP 状态码。
	}{
		{"valid", `{"name":"ok"}`, false, http.StatusOK},
		{"unknown", `{"extra":1}`, false, http.StatusBadRequest},
		{"second_value", `{"name":"a"} {"name":"b"}`, false, http.StatusBadRequest},
		{"malformed", `{"name":`, false, http.StatusBadRequest},
		{"oversized", `{"name":"` + strings.Repeat("a", int(DefaultBodyLimit)) + `"}`, false, http.StatusRequestEntityTooLarge},
		{"chunked", `{"name":"` + strings.Repeat("a", int(DefaultBodyLimit)) + `"}`, true, http.StatusRequestEntityTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			router := testRouter(&logs, func(ctx *gin.Context) {
				var input struct {
					Name string `json:"name"` /* Name 是测试用的唯一合法字段。 */
				}
				if BindJSON(ctx, &input) {
					Success(ctx, http.StatusOK, input)
				}
			})
			request := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(test.body))
			if test.stream {
				request.ContentLength = -1
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("状态码 %d，期望 %d", response.Code, test.status)
			}
		})
	}
}
