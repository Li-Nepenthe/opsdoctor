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

// DefaultBodyLimit 是 JSON 接口的 1 MiB 预算，足够容纳小型业务请求，避免无界读取耗尽内存。
const DefaultBodyLimit int64 = 1 << 20

// maximumRequestIDBytes 限制客户端附带的标识长度，防止把大量内容带进每一条日志。
const maximumRequestIDBytes = 64

// RequestID 返回当前上下文里的标识；须先安装 RequestIDs 中间件，否则返回空字符串。
func RequestID(ctx *gin.Context) string {
	return logging.FromContext(ctx.Request.Context()).RequestID
}

// RequestIDs 校验或生成请求标识，并记录安全的访问摘要；logger 必须非 nil。
// 返回的中间件必须先于 Recovery 安装；每个请求只修改自己的 context，可处理并发请求。
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
		// 使用路由模板，不写原始 URL/query，避免密码等查询参数出现在日志里。
		logger.InfoContext(ctx.Request.Context(), "HTTP 请求完成", "method", ctx.Request.Method,
			"route", ctx.FullPath(), "status", ctx.Writer.Status(), "duration_ms", time.Since(started).Milliseconds())
	}
}

// validRequestID 只接受简单 ASCII 标识；无效输入会被替换，而不是原样带入日志或响应头。
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

// Recovery 捕获当前请求 goroutine 的 panic，输出不含内部细节的 500；logger 必须非 nil。
// 它不能捕获业务自行启动的 goroutine 的 panic；已经写出的流式响应也无法撤回。
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		defer func() {
			if recover() == nil {
				return
			}
			// panic 可能包含 SQL、密码或请求体，只记录固定分类，不记录其值或请求转储。
			logger.ErrorContext(ctx.Request.Context(), "HTTP 请求异常", "error", "panic")
			if !ctx.Writer.Written() {
				Failure(ctx, http.StatusInternalServerError, "INTERNAL", "内部错误")
			}
			ctx.Abort()
		}()
		ctx.Next()
	}
}

// BodyLimit 给每个请求安装有上限的 Reader；limit 必须大于 0，通常使用 DefaultBodyLimit。
// 对已知长度提前拒绝；分块传输仍由 MaxBytesReader 限制，调用方应使用 BindJSON 处理读取错误。
func BodyLimit(limit int64) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if limit <= 0 {
			Failure(ctx, http.StatusInternalServerError, "INTERNAL", "内部错误")
			return
		}
		if ctx.Request.ContentLength > limit {
			Failure(ctx, http.StatusRequestEntityTooLarge, "INVALID_ARGUMENT", "请求体过大")
			return
		}
		ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, limit)
		ctx.Next()
	}
}

// BindJSON 严格解析一个 JSON 值到非 nil 指针 target；失败时已写出安全响应并中断请求链。
// 必须配合 BodyLimit；读取阻塞由服务器超时/context 控制，不能在多个 goroutine 共享 Gin context。
func BindJSON(ctx *gin.Context, target any) bool {
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(target)
	if err == nil {
		// 第二次解码必须遇到 EOF，防止悄悄忽略请求中追加的另一个 JSON 值。
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

// Success 写出契约规定的成功响应；status 由接口决定（例如查询 200、创建 201）。
// data 必须可以编码为 JSON；此函数同步写出响应，不负责业务错误映射。
func Success(ctx *gin.Context, status int, data any) {
	ctx.JSON(status, gin.H{"code": "OK", "data": data, "request_id": RequestID(ctx)})
}

// Failure 写出公开错误并中断当前处理链；code/message 必须是服务定义的安全文本，不能直接传 err.Error()。
// 它不记录内部错误；调用方可用同一个 request_id 记录脱敏后的错误分类。
func Failure(ctx *gin.Context, status int, code, message string) {
	ctx.AbortWithStatusJSON(status, gin.H{"code": code, "message": message, "request_id": RequestID(ctx)})
}
