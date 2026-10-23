// 本文件：检查商品接口的错误分类与库存展示。
//
// handler 使用真实商品流程和内存依赖，证明超时与库存降级属于不同的响应路径。
//
// 主要内容：
//   - TestProductHTTP：逐项核对编号错误、不存在、查询超时、内部故障和正常响应。
//
// 阅读建议：商品数据库超时返回 504，只有库存展示失败才返回降级的 200。

package product

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestProductHTTP 防止把查询超时混成 500，也防止降级时把未知库存写成零。
func TestProductHTTP(t *testing.T) {
	for _, testcase := range []struct {
		name     string // 用中文说明这一行的输入条件。
		id       string // URL 中的商品编号。
		queryErr error  // 商品数据库返回的错误。
		stockErr error  // 仅影响库存展示的错误。
		status   int    // 契约中的 HTTP 状态码。
		code     string // 契约中的稳定错误码。
	}{
		{"编号不合法", "0", nil, nil, 400, "INVALID_ARGUMENT"},                       // 无效编号不进入存储。
		{"商品不存在", "1", ErrNotFound, nil, 404, "PRODUCT_NOT_FOUND"},              // 空值缓存也保持 404。
		{"商品查询超时", "1", context.DeadlineExceeded, nil, 504, "UPSTREAM_TIMEOUT"}, // 没有商品，不能降级。
		{"商品查询取消", "1", context.Canceled, nil, 504, "UPSTREAM_TIMEOUT"},         // 取消仍使用统一超时分类。
		{"数据库内部失败", "1", errors.New("测试内部错误"), nil, 500, "INTERNAL"},            // 不泄露内部错误原文。
		{"库存不可用", "1", nil, errors.New("库存断开"), 200, "OK"},                      // 商品已查到，允许省略库存数值。
		{"正常零库存", "1", nil, nil, 200, "OK"},                                     // 零是已知数值，不能被省略。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			service := NewService(&fakeRepository{value: Product{ID: 1, PriceCents: 100}, err: testcase.queryErr}, &memoryCache{values: make(map[int64]cachedValue)}, fakeStock{err: testcase.stockErr}, false, quietLogger())
			router := gin.New()
			router.GET("/products/:id", NewHandler(service).Get)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("GET", "/products/"+testcase.id, nil))
			var body struct {
				Code string `json:"code"` // 只解码断言需要的字段。
				Data Detail `json:"data"` // 保留库存指针，区分未知和零。
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != testcase.status || body.Code != testcase.code {
				t.Fatalf("响应不符：%d %s", response.Code, response.Body.String())
			}
			// 成功时进一步检查可选库存字段，避免状态码正确但数据含义错误。
			if testcase.status == 200 && ((body.Data.Stock.Available == nil) != (testcase.stockErr != nil)) {
				t.Fatalf("库存数值与降级状态不符：%s", response.Body.String())
			}
		})
	}
}
