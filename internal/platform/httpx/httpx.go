// 本文件：请求进入业务处理前，路由通过这里的中间件设置请求标识、异常恢复和正文上限。
// 从 RequestIDs 读起，再看 BindJSON 如何拒绝有多余内容的请求。

package httpx

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/logging"
	"github.com/gin-gonic/gin"
)

// DefaultBodyLimit 将普通 JSON 请求的读取预算限制为 1 MiB。
const DefaultBodyLimit int64 = 1 << 20

const maximumRequestIDBytes = 64

// RequestID 返回本次请求的日志标识。尚未安装 RequestIDs 时返回空字符串。
func RequestID(ctx *gin.Context) string {
	return logging.FromContext(ctx.Request.Context()).RequestID
}

// RequestIDs 将同一标识写入响应和访问日志。它必须先于 Recovery 安装，使异常响应也带有这次请求的标识。
func RequestIDs(logger *slog.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		id := ctx.GetHeader("X-Request-ID")
		if !validRequestID(id) {
			id = rand.Text()
		}
		fields := logging.FromContext(ctx.Request.Context())
		fields.RequestID = id
		ctx.Request = ctx.Request.WithContext(logging.WithFields(ctx.Request.Context(), fields))
		ctx.Header("X-Request-ID", id)
		started := time.Now()
		ctx.Next()
		// 记录注册时的路由模板。原始 URL 和查询串可能带有密码。
		logger.InfoContext(ctx.Request.Context(), "HTTP 请求完成", "method", ctx.Request.Method,
			"route", ctx.FullPath(), "status", ctx.Writer.Status(), "duration_ms", time.Since(started).Milliseconds(), "user_agent", clientAgent(ctx.GetHeader("User-Agent")))
	}
}

// 客户端提供的标识会进日志和响应头，所以只接受有限长度的 ASCII 字母、数字、短横线和下划线。
func validRequestID(id string) bool {
	if len(id) == 0 || len(id) > maximumRequestIDBytes {
		return false
	}
	for _, char := range id {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_') {
			return false
		}
	}
	return true
}

// Recovery 捕获当前请求中的 panic。若响应尚未写出，就返回通用 500 错误。业务另起的 goroutine 须自行处理 panic。
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		defer func() {
			if recover() == nil {
				return
			}
			// panic 的值可能含有 SQL、密码或请求正文，日志只记固定分类。
			logger.ErrorContext(ctx.Request.Context(), "HTTP 请求异常", "error", "panic")
			// 流式响应可能已经写出一部分，这时只能结束处理，无法替换其状态码或正文。
			if !ctx.Writer.Written() {
				Failure(ctx, http.StatusInternalServerError, "INTERNAL", "内部错误")
			}
			ctx.Abort()
		}()
		ctx.Next()
	}
}

// BodyLimit 限制每份正文的字节数。limit 须大于零，并应在 BindJSON 前安装。
func BodyLimit(limit int64) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		// 上限由服务配置，写错应返回服务错误。
		if limit <= 0 {
			Failure(ctx, http.StatusInternalServerError, "INTERNAL", "内部错误")
			return
		}
		if ctx.Request.ContentLength > limit {
			Failure(ctx, http.StatusRequestEntityTooLarge, "INVALID_ARGUMENT", "请求体过大")
			return
		}
		// 分块请求可能没有 Content-Length，还要在实际读取正文时限制大小。
		ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, limit)
		ctx.Next()
	}
}

// BindJSON 将单个 JSON 值读入 target 指向的结构，拒绝未知字段。返回 false 时错误响应已经写出。
func BindJSON(ctx *gin.Context, target any) bool {
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(target)
	if err == nil {
		// 首个值合法仍可能夹带第二份数据。只有再次解码读到 EOF，才能接受整个请求。
		var extra any
		err = decoder.Decode(&extra)
		if errors.Is(err, io.EOF) {
			return true
		}
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		Failure(ctx, http.StatusRequestEntityTooLarge, "INVALID_ARGUMENT", "请求体过大")
		return false
	}
	Failure(ctx, http.StatusBadRequest, "INVALID_ARGUMENT", "请求必须是符合接口约定的单个 JSON 值")
	return false
}

// Success 将业务结果和请求标识一起写入成功响应。
func Success(ctx *gin.Context, status int, data any) {
	ctx.JSON(status, gin.H{"code": "OK", "data": data, "request_id": RequestID(ctx)})
}

// Failure 中断请求并写出供客户端查看的错误，code 和 message 应使用可公开的文字。
func Failure(ctx *gin.Context, status int, code, message string) {
	ctx.AbortWithStatusJSON(status, gin.H{"code": code, "message": message, "request_id": RequestID(ctx)})
}
