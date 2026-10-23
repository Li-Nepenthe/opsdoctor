// 本文件：验证订单 HTTP 契约和严格输入解析。
//
// 直接让 Gin 处理内存请求，不开启监听端口。业务服务用替身，测试聚焦响应格式与参数传递。
//
// 主要内容：
//   - stubHTTPService：记录 handler 传入的参数并返回指定结果。
//   - handlerRouter：安装真实请求标识、恢复和请求体限制中间件。
//   - TestCreateHTTP：检查创建结果、错误码和损坏请求。
//   - TestQueryHTTP：检查详情、分页默认值和超范围参数。
//
// 阅读建议：先看原始 JSON 输入，再看断言为什么要检查 request_id。

package order

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/httpx"
	"github.com/gin-gonic/gin"
)

// stubHTTPService 保存最后的输入，便于证明 HTTP 参数传到了业务层。
type stubHTTPService struct {
	err        error         // 指定业务错误，不在 HTTP 层重新模拟数据库失败。
	calls      int           // 解析失败时不应调用业务。
	request    CreateRequest // 下单 JSON 解析后的值。
	key        string        // 幂等键必须原样传递。
	page, size int           // 记录分页默认值。
}

// Create 返回稳定的结果，错误文本故意带内部字样，检查响应没有泄露它。
func (service *stubHTTPService) Create(_ context.Context, request CreateRequest, key string) (Created, error) {
	service.calls++
	service.request, service.key = request, key
	return Created{OrderID: 1, AmountCents: 200, Status: "created"}, service.err
}

// Get 在查询测试中只提供编号，其他详情字段由真实仓储测试覆盖。
func (service *stubHTTPService) Get(_ context.Context, id int64) (Order, error) {
	service.calls++
	return Order{OrderID: id}, service.err
}

// List 借用真实 service 的参数校验，再返回空列表，避免替身把非法分页也当成成功。
func (service *stubHTTPService) List(ctx context.Context, user int64, page, size int) (Page, error) {
	service.calls++
	service.page, service.size = page, size
	validation := &Service{repository: &fakeRepository{}}
	return validation.List(ctx, user, page, size)
}

// handlerRouter 使用内存调用 ServeHTTP，不启动服务器或连接外部服务。
func handlerRouter(service *stubHTTPService) *gin.Engine {
	router := gin.New()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router.Use(httpx.RequestIDs(logger), httpx.Recovery(logger), httpx.BodyLimit(httpx.DefaultBodyLimit))
	handler := NewHandler(service)
	router.POST("/orders", handler.Create)
	router.GET("/orders/:id", handler.Get)
	router.GET("/orders", handler.List)
	return router
}

// TestCreateHTTP 覆盖幂等三种响应和严格 JSON，保证客户端能区分等一等与人工核对。
func TestCreateHTTP(t *testing.T) {
	const validBody = `{"user_id":1,"sku_id":2,"quantity":2}` // 直接使用契约中的三个字段。
	for _, testcase := range []struct {
		name      string // 说明当前输入或业务结果。
		body, key string // 原始 HTTP 输入。
		err       error  // 替身业务结果。
		status    int    // 期望 HTTP 状态码。
		code      string // 期望公开错误码。
		calls     int    // 是否应该进入业务层。
	}{
		{"创建成功", validBody, "request-1", nil, 201, "OK", 1},                                                 // 新建与成功回放都用 201。
		{"处理中", validBody, "request-1", ErrInProgress, 409, "REQUEST_IN_PROGRESS", 1},                       // 客户端可稍后查询。
		{"待核对", validBody, "request-1", ErrOutcomeUnknown, 409, "REQUEST_OUTCOME_UNKNOWN", 1},               // 不能换键或重做。
		{"Redis 不可用", validBody, "request-1", ErrIdempotencyUnavailable, 503, "UPSTREAM_UNAVAILABLE", 1},    // 不绕开幂等保护。
		{"内部错误", validBody, "request-1", errors.New("private-database-details"), 500, "INTERNAL", 1},        // 不泄露原始内部文本。
		{"额外字段", `{"user_id":1,"sku_id":2,"quantity":2,"price":1}`, "key", nil, 400, "INVALID_ARGUMENT", 0}, // 客户端不能覆盖价格。
		{"尾随对象", validBody + ` {}`, "key", nil, 400, "INVALID_ARGUMENT", 0},                                 // 一个请求只能有一个对象。
		{"数量溢出", `{"user_id":1,"sku_id":2,"quantity":2147483648}`, "key", nil, 400, "INVALID_ARGUMENT", 0},  // int32 边界在解析时拒绝。
		{"键不合法", validBody, "key with space", nil, 400, "INVALID_ARGUMENT", 0},                              // 不接受契约外字符。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			service := &stubHTTPService{err: testcase.err}
			request := httptest.NewRequest("POST", "/orders", strings.NewReader(testcase.body))
			request.Header.Set("Idempotency-Key", testcase.key)
			response := httptest.NewRecorder()
			handlerRouter(service).ServeHTTP(response, request)
			var envelope struct {
				Code      string `json:"code"`       // 对外稳定分类。
				RequestID string `json:"request_id"` // 每次响应必须有可追踪标识。
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if response.Code != testcase.status || envelope.Code != testcase.code || envelope.RequestID == "" || service.calls != testcase.calls {
				t.Fatalf("status=%d body=%s calls=%d", response.Code, response.Body.String(), service.calls)
			}
			if strings.Contains(response.Body.String(), "private-database-details") {
				t.Fatal("泄露内部错误")
			}
		})
	}
}

// TestQueryHTTP 验证默认分页与无效查询，空订单列表必须输出数组。
func TestQueryHTTP(t *testing.T) {
	for _, testcase := range []struct {
		path   string // 一个路径对应一个参数边界。
		status int    // 请求能否被接受。
	}{
		{"/orders/1", 200},                                                // 正整数详情编号。
		{"/orders/no", 400},                                               // 文本编号不能传入数据库。
		{"/orders?user_id=1", 200},                                        // 使用第一页、每页二十条。
		{"/orders?user_id=1&page=0", 400},                                 // 页码从一开始。
		{"/orders?user_id=1&page_size=101", 400},                          // 每页最多一百条。
		{"/orders?user_id=1&page=9223372036854775807&page_size=100", 400}, // 偏移量乘法不能溢出。
		{"/orders", 400},                                                  // 用户编号不能省略。
	} {
		t.Run(testcase.path, func(t *testing.T) {
			service := &stubHTTPService{}
			response := httptest.NewRecorder()
			handlerRouter(service).ServeHTTP(response, httptest.NewRequest("GET", testcase.path, nil))
			if response.Code != testcase.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if testcase.path == "/orders?user_id=1" && (service.page != 1 || service.size != 20 || !strings.Contains(response.Body.String(), `"items":[]`)) {
				t.Fatalf("分页默认值不对：%s", response.Body.String())
			}
		})
	}
}
