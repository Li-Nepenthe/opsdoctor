// 本文件：HTTP 请求进入业务前后的通用保护。
//
// 这里统一请求标识、请求体限制、JSON 解析和 panic 恢复，让业务 handler 专注于业务。
// 先读 RequestIDs 与 Recovery 的安装顺序，再读 BodyLimit 和 BindJSON 如何配合。Success/Failure 是最后的响应出口。
//
// 主要内容：
//   - RequestID：返回当前上下文里的标识。须先安装 RequestIDs 中间件，否则返回空字符串。
//   - RequestIDs：校验或生成请求标识，并记录安全的访问摘要。logger 必须非 nil。
//   - validRequestID：只接受简单 ASCII 标识。无效输入会被替换，而不是原样带入日志或响应头。
//   - Recovery：捕获当前请求 goroutine 的 panic，输出不含内部细节的 500。logger 必须非 nil。
//   - BodyLimit：给每个请求安装有上限的 Reader。limit 必须大于 0，通常使用 DefaultBodyLimit。
//   - BindJSON：严格解析一个 JSON 值到非 nil 指针 target。失败时已写出安全响应并中断请求链。
//   - Success：写出契约规定的成功响应。status 由接口决定（例如查询 200、创建 201）。
//   - Failure：写出公开错误并中断当前处理链。code/message 必须是服务定义的安全文本，不能直接传 err.Error()。

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

// RequestID 返回当前上下文里的标识。须先安装 RequestIDs 中间件，否则返回空字符串。
func RequestID(ctx *gin.Context) string {
	return logging.FromContext(ctx.Request.Context()).RequestID
}

// RequestIDs 校验或生成请求标识，并记录安全的访问摘要。logger 必须非 nil。
// 必须先于 Recovery 安装，这样 panic 恢复后的失败日志和响应也能带上同一个 request_id。
func RequestIDs(logger *slog.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		id := ctx.GetHeader("X-Request-ID")
		if !validRequestID(id) {
			// 客户端没给标识或给了危险字符时重新生成，不能直接把未经检查的输入放进日志。
			id = rand.Text()
		}
		fields := logging.FromContext(ctx.Request.Context())
		fields.RequestID = id
		ctx.Request = ctx.Request.WithContext(logging.WithFields(ctx.Request.Context(), fields))
		ctx.Header("X-Request-ID", id)
		started := time.Now()
		// Gin 的 Next 执行后面的中间件和处理函数。它返回后才能记录最终状态码和完整耗时。
		ctx.Next()
		// 使用路由模板，不写原始 URL/query，避免密码等查询参数出现在日志里。
		logger.InfoContext(ctx.Request.Context(), "HTTP 请求完成", "method", ctx.Request.Method,
			"route", ctx.FullPath(), "status", ctx.Writer.Status(), "duration_ms", time.Since(started).Milliseconds(), "user_agent", clientAgent(ctx.GetHeader("User-Agent")))
	}
}

// validRequestID 只接受简单 ASCII 标识。无效输入会被替换，而不是原样带入日志或响应头。
func validRequestID(id string) bool {
	if len(id) == 0 || len(id) > maximumRequestIDBytes {
		// 缺失时需要生成标识，过长时需要限制日志大小，两种情况都不能原样采用。
		return false
	}
	for _, char := range id {
		// 只允许字母、数字、短横线和下划线，排除换行等会干扰日志和响应头的字符。
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_') {
			return false
		}
	}
	return true
}

// Recovery 捕获当前请求 goroutine 的 panic，输出不含内部细节的 500。logger 必须非 nil。
// 它不能捕获业务自行启动的 goroutine 的 panic。已经写出的流式响应也无法撤回。
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		defer func() {
			if recover() == nil {
				// recover 必须在 defer 中调用。正常返回时没有 panic，直接结束清理函数。
				return
			}
			// panic 可能包含 SQL、密码或请求体，只记录固定分类，不记录其值或请求转储。
			logger.ErrorContext(ctx.Request.Context(), "HTTP 请求异常", "error", "panic")
			if !ctx.Writer.Written() {
				// 响应头或正文一旦发出就不能撤回，只在尚未写出时替换成统一的 500 响应。
				Failure(ctx, http.StatusInternalServerError, "INTERNAL", "内部错误")
			}
			ctx.Abort()
			// Abort 防止继续执行剩余处理函数。它不会撤回已经写出的响应内容。
		}()
		ctx.Next()
	}
}

// BodyLimit 给每个请求安装有上限的 Reader。limit 必须大于 0，通常使用 DefaultBodyLimit。
// 对已知长度提前拒绝。分块传输仍由 MaxBytesReader 限制，调用方应使用 BindJSON 处理读取错误。
func BodyLimit(limit int64) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if limit <= 0 {
			// 这是服务组装时的参数错误，返回 500。不能归咎为客户端请求格式错误。
			Failure(ctx, http.StatusInternalServerError, "INTERNAL", "内部错误")
			return
		}
		if ctx.Request.ContentLength > limit {
			// 已知长度已经超限，无需再读取正文，尽早拒绝能少占用带宽和内存。
			Failure(ctx, http.StatusRequestEntityTooLarge, "INVALID_ARGUMENT", "请求体过大")
			return
		}
		// 没有 Content-Length 的流式请求也必须限量。包装 Reader 后，每次实际读取都会累计字节数。
		ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, limit)
		ctx.Next()
	}
}

// BindJSON 严格解析一个 JSON 值到非 nil 指针 target。失败时已写出安全响应并中断请求链。
// 必须配合 BodyLimit。读取阻塞由服务器超时/context 控制，不能在多个 goroutine 共享 Gin context。
func BindJSON(ctx *gin.Context, target any) bool {
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	// 未知字段往往表示参数名拼错。禁止静默忽略，让客户端及时发现问题。
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
		// As 按错误类型查找（也能穿过包装），把真正的超限和普通 JSON 语法错误区分开。
		Failure(ctx, http.StatusRequestEntityTooLarge, "INVALID_ARGUMENT", "请求体过大")
		return false
	}
	Failure(ctx, http.StatusBadRequest, "INVALID_ARGUMENT", "请求必须是符合接口约定的单个 JSON 值")
	return false
}

// Success 写出契约规定的成功响应。status 由接口决定（例如查询 200、创建 201）。
// handler 先决定业务结果，再调用这里的统一外壳，避免各接口拼出不同格式。
func Success(ctx *gin.Context, status int, data any) {
	ctx.JSON(status, gin.H{"code": "OK", "data": data, "request_id": RequestID(ctx)})
}

// Failure 写出公开错误并中断当前处理链。code/message 必须是服务定义的安全文本，不能直接传 err.Error()。
// 它不记录内部错误。调用方可用同一个 request_id 记录脱敏后的错误分类。
func Failure(ctx *gin.Context, status int, code, message string) {
	ctx.AbortWithStatusJSON(status, gin.H{"code": code, "message": message, "request_id": RequestID(ctx)})
}
