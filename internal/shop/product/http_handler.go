// 本文件：接收商品详情请求，把业务结果转换成统一 HTTP 响应。
//
// 请求从 Gin 路由进入这里，再交给 service。这里不读取数据库或 Redis。
//
// 主要内容：
//   - Handler、NewHandler：保存商品服务依赖。
//   - Get：验证 URL 中的编号，并转换商品查询的错误。
//
// 阅读建议：先看编号解析，再看不存在和内部错误为什么返回不同状态。

package product

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/httpx"
	"github.com/gin-gonic/gin"
)

// Handler 只保存业务服务，请求参数留在每次调用的局部变量中。
type Handler struct{ service *Service } // service 决定缓存和降级流程。

// NewHandler 在 app 装配路由时创建，避免每次请求重复创建服务。
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
