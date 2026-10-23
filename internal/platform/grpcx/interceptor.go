// 本文件：给每个一元 RPC 加日志和 panic 恢复。
//
// 拦截器类似围绕方法调用的包装层：Logging 在外，Recovery 在内，才能记到恢复后的最终状态。
// 先看 Logging 的 handler 调用，再看 Recovery 的具名返回值与 defer；请求体和 panic 原文不会进入日志。

package grpcx

import (
	"context"
	"crypto/rand"
	"log/slog"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Logging 返回最外层拦截器，给每次 RPC 建立关联 ID，并记录内层恢复 panic 后的最终状态。
// logger 必须非 nil；不记录请求体、原始错误或 metadata，避免凭据进入日志。
func Logging(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// rand.Text 生成适合作为标识的随机文本；request_id 不承担鉴权作用。
		fields := logging.FromContext(ctx)
		fields.RequestID = rand.Text()
		ctx = logging.WithFields(ctx, fields)
		started := time.Now()
		response, err := handler(ctx, request)
		attributes := []any{"method", info.FullMethod, "code", status.Code(err).String(), "duration_ms", time.Since(started).Milliseconds()}
		if err != nil {
			// 状态码足以定位失败类别；业务错误详情可能带数据库信息，不能原样记录。
			attributes = append(attributes, "error", status.Code(err).String())
			logger.WarnContext(ctx, "gRPC 请求失败", attributes...)
		} else {
			logger.InfoContext(ctx, "gRPC 请求完成", attributes...)
		}
		return response, err
	}
}

// Recovery 放在 Logging 内层，把业务 panic 转成 INTERNAL；只恢复当前请求的 goroutine。
func Recovery(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (response any, result error) {
		// response/result 是具名返回值：return 先赋值，再运行 defer，所以 defer 可以替换最终结果。
		// recover 必须直接在 defer 函数中调用；panic 的值可能含密码，故意不输出它。
		defer func() {
			if recover() != nil {
				logger.ErrorContext(ctx, "gRPC 请求发生 panic，已恢复", "method", info.FullMethod, "error", "panic")
				response = nil
				result = status.Error(codes.Internal, "服务内部错误")
			}
		}()
		return handler(ctx, request)
	}
}
