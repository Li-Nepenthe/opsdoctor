// 本文件：一元 RPC 的访问日志包住 panic 恢复层，才能记录恢复后的最终状态。
// 从 Logging 调用内层 handler 的位置，可以看到 Recovery 返回的状态如何进入日志。

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

// Logging 为每次 RPC 生成关联 ID 并记录最终状态。
// 日志只含方法、耗时和状态，请求体、原始错误和 metadata 可能携带凭据。
func Logging(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		fields := logging.FromContext(ctx)
		fields.RequestID = rand.Text()
		ctx = logging.WithFields(ctx, fields)
		started := time.Now()
		response, err := handler(ctx, request)
		attributes := []any{"method", info.FullMethod, "code", status.Code(err).String(), "duration_ms", time.Since(started).Milliseconds()}
		if err != nil {
			// 状态码足以定位失败类别。业务错误详情可能带数据库信息，不能原样记录。
			attributes = append(attributes, "error", status.Code(err).String())
			logger.WarnContext(ctx, "gRPC 请求失败", attributes...)
		} else {
			logger.InfoContext(ctx, "gRPC 请求完成", attributes...)
		}
		return response, err
	}
}

// Recovery 只恢复当前 RPC 所在 goroutine 的 panic，向客户端返回固定的 Internal 错误。
func Recovery(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (response any, result error) {
		// panic 的值可能包含凭据，不能写入日志。
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
