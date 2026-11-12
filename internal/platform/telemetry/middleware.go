// 本文件：在 HTTP 和 gRPC 请求结束时记录耗时与状态。指标名由项目固定，看板和告警才能在更换调用链插件后继续工作。

package telemetry

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// HTTPMiddleware 放在恢复和限流之前，这样被拒绝的请求也能进入总请求数。
func (metrics *Metrics) HTTPMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		started := time.Now()
		ctx.Next()
		route := ctx.FullPath()
		if route == "" {
			// 未匹配路由统一归类，扫描者请求一万个不同地址也只产生一组序列。
			route = "unmatched"
		}
		method := ctx.Request.Method
		switch method {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "CONNECT", "TRACE":
		default:
			method = "OTHER" // HTTP 方法也是外部输入，未知值必须合并。
		}
		statusCode := ctx.Writer.Status()
		metrics.HTTP.WithLabelValues(method, route, strconv.Itoa(statusCode)).Observe(time.Since(started).Seconds())
		if statusCode == http.StatusTooManyRequests {
			metrics.RateRejected.WithLabelValues(route).Inc()
		}
	}
}

// Server 包在恢复拦截器外层，panic 转成 Internal 后也会被记录。
func (metrics *Metrics) Server() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		started := time.Now()
		response, err := handler(ctx, request)
		metrics.RPCServer.WithLabelValues(info.FullMethod, rpcCode(err)).Observe(time.Since(started).Seconds())
		return response, err
	}
}

// Client 的 target 使用 app 提供的服务名称，不能直接使用包含任意地址的请求参数。
func (metrics *Metrics) Client(target string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, request, response any, connection *grpc.ClientConn, invoke grpc.UnaryInvoker, options ...grpc.CallOption) error {
		started := time.Now()
		err := invoke(ctx, method, request, response, connection, options...)
		metrics.RPCClient.WithLabelValues(target, method, rpcCode(err)).Observe(time.Since(started).Seconds())
		return err
	}
}

// rpcCode 避免本地 deadline 被笼统地记为 Unknown，方便区分超时与服务端故障。
func rpcCode(err error) string {
	if errors.Is(err, context.Canceled) {
		return codes.Canceled.String()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return codes.DeadlineExceeded.String()
	}
	return status.Code(err).String()
}
