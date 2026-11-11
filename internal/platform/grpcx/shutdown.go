// 本文件：gRPC 服务排空在途请求时仍要遵守进程的清理时限。
// 服务清理时调用 Shutdown，先等待 GracefulStop，再在预算耗尽时终止仍在执行的 RPC。

package grpcx

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
)

// Shutdown 停止接收新 RPC，等待在途请求完成。
// 调用方须传入有限的 deadline；返回时后台停机操作已经结束。
func Shutdown(ctx context.Context, server *grpc.Server) error {
	done := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		// GracefulStop 本身不接收 context，需要显式强制关闭才能给停机时间设上界。
		server.Stop()
		<-done
		return fmt.Errorf("gRPC 未在预算内排空，已强制关闭：%w", ctx.Err())
	}
}
