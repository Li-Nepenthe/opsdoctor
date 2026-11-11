// 本文件：解析下单和查询请求，把业务结果或错误转换成约定的 HTTP 响应。
// Create 先检查幂等键与 JSON 格式，再由订单服务执行业务；重复请求的状态决定客户端能否重试。

package order

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/httpx"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/inventory"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/product"
	"github.com/gin-gonic/gin"
)

// HTTPService 让 handler 测试可以只检查传输契约，不必顺便运行整个下单流程。
type HTTPService interface {
	Create(context.Context, CreateRequest, string) (Created, error)
	Get(context.Context, int64) (Order, error)
	List(context.Context, int64, int, int) (Page, error)
}

// Handler 处理创建订单、订单详情和用户订单列表的 HTTP 请求。
type Handler struct{ service HTTPService }

// NewHandler 把三条订单路由接到同一个业务服务。
func NewHandler(service HTTPService) *Handler { return &Handler{service: service} }

// Create 不接受多个幂等键，避免代理与服务对“到底用哪一个”产生不同理解。
func (handler *Handler) Create(ctx *gin.Context) {
	keys := ctx.Request.Header.Values("Idempotency-Key")
	key := ctx.GetHeader("Idempotency-Key")
	if len(keys) > 1 || (len(keys) == 1 && key == "") || !validKey(key) {
		respondError(ctx, ErrInvalidArgument)
		return
	}
	var request CreateRequest
	if !httpx.BindJSON(ctx, &request) {
		return
	}
	// 业务参数正数与金额上界由 service 再检查，HTTP 不是唯一可能的调用入口。
	result, err := handler.service.Create(ctx.Request.Context(), request, key)
	if err != nil {
		respondError(ctx, err)
		return
	}
	httpx.Success(ctx, http.StatusCreated, result)
}

// Get 使用 ParseInt 拒绝溢出的编号，不能把它截断后查询另一个订单。
func (handler *Handler) Get(ctx *gin.Context) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		respondError(ctx, ErrInvalidArgument)
		return
	}
	result, err := handler.service.Get(ctx.Request.Context(), id)
	if err != nil {
		respondError(ctx, err)
		return
	}
	httpx.Success(ctx, http.StatusOK, result)
}

// List 使用契约中的默认页码和页大小，不接受无法解析的字符串。
func (handler *Handler) List(ctx *gin.Context) {
	userID, err := strconv.ParseInt(ctx.Query("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		respondError(ctx, ErrInvalidArgument)
		return
	}
	page, err := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	if err != nil {
		respondError(ctx, ErrInvalidArgument)
		return
	}
	pageSize, err := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	if err != nil {
		respondError(ctx, ErrInvalidArgument)
		return
	}
	// service 还会检查正数、最大页大小与偏移量溢出，避免其他调用者绕过规则。
	result, err := handler.service.List(ctx.Request.Context(), userID, page, pageSize)
	if err != nil {
		respondError(ctx, err)
		return
	}
	httpx.Success(ctx, http.StatusOK, result)
}

// respondError 不向客户端暴露内部错误文本。可处理的业务分类使用稳定错误码。
func respondError(ctx *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, "INTERNAL", "订单处理失败"
	// 先处理业务拒绝和需要人工核对的状态，最后才判断通用的下游超时。
	switch {
	case errors.Is(err, ErrInvalidArgument), errors.Is(err, product.ErrInvalidArgument), errors.Is(err, inventory.ErrInvalidArgument):
		status, code, message = http.StatusBadRequest, "INVALID_ARGUMENT", "订单参数不合法"
	case errors.Is(err, product.ErrNotFound), errors.Is(err, inventory.ErrNotFound):
		status, code, message = http.StatusNotFound, "PRODUCT_NOT_FOUND", "商品不存在"
	case errors.Is(err, ErrNotFound):
		status, code, message = http.StatusNotFound, "ORDER_NOT_FOUND", "订单不存在"
	case errors.Is(err, inventory.ErrInsufficientStock):
		status, code, message = http.StatusConflict, "INSUFFICIENT_STOCK", "库存不足"
	case errors.Is(err, ErrInProgress):
		status, code, message = http.StatusConflict, "REQUEST_IN_PROGRESS", "同一请求还在处理中"
	case errors.Is(err, ErrOutcomeUnknown):
		status, code, message = http.StatusConflict, "REQUEST_OUTCOME_UNKNOWN", "下单结果待核对，请勿换键重试"
	case errors.Is(err, ErrIdempotencyUnavailable), errors.Is(err, inventory.ErrUnavailable):
		status, code, message = http.StatusServiceUnavailable, "UPSTREAM_UNAVAILABLE", "下游服务暂时不可用"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		status, code, message = http.StatusGatewayTimeout, "UPSTREAM_TIMEOUT", "下游处理超时或已取消"
	}
	httpx.Failure(ctx, status, code, message)
}
