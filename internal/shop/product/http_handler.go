// 本文件：商品详情接口区分编号无效、商品缺失和查询失败，库存不可用时仍可展示商品。
// 订单服务将详情路由接到 Get，由它把业务结果映射为 HTTP 响应。

package product

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/httpx"
	"github.com/gin-gonic/gin"
)

// Handler 接收商品详情请求。
type Handler struct{ service *Service }

// NewHandler 把商品查询服务接到 HTTP 路由。
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// Get 拒绝非正整数编号，避免无意义的输入继续占用数据库和 RPC 资源。
func (handler *Handler) Get(ctx *gin.Context) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.Failure(ctx, http.StatusBadRequest, "INVALID_ARGUMENT", "商品编号必须是正整数")
		return
	}
	detail, err := handler.service.Detail(ctx.Request.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.Failure(ctx, http.StatusNotFound, "PRODUCT_NOT_FOUND", "商品不存在")
		return
	}
	// 商品本身没查出来时无法降级。预算耗尽使用统一的超时码，不能混成普通内部故障。
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		httpx.Failure(ctx, http.StatusGatewayTimeout, "UPSTREAM_TIMEOUT", "商品查询超时或已取消")
		return
	}
	// 数据库错误可能包含连接信息。响应只给稳定错误码，不把内部错误文本原样发出去。
	if err != nil {
		httpx.Failure(ctx, http.StatusInternalServerError, "INTERNAL", "商品查询失败")
		return
	}
	httpx.Success(ctx, http.StatusOK, detail)
}
