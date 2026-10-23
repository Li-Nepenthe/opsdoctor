// 本文件：给 gRPC 的优雅停机加上总预算。
//
// GracefulStop 会等在途请求，但没有 context 参数，所以 Shutdown 在预算耗尽时补调 Stop。
// 先看 done 的关闭者，再看超时分支如何等待后台调用结束，防止清理函数自己留下 goroutine。
//
// 主要内容：
//   - Shutdown：先停止接收新 RPC 并等待在途请求，清理预算耗尽时用 Stop 取消剩余 RPC。

package grpcx

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
)

// Shutdown 先停止接收新 RPC 并等待在途请求，清理预算耗尽时用 Stop 取消剩余 RPC。
// 调用方必须提供有限 deadline。返回前等待内部 goroutine 退出，不把清理工作遗留在后台。
func Shutdown(ctx context.Context, server *grpc.Server) error {
	// done 只传递“已结束”，不传值。后台 goroutine 是唯一关闭者，本函数是唯一接收者，无需缓冲。
	done := make(chan struct{})
	go func() {
		// GracefulStop 正常排空会返回。超时分支调用 Stop 后也会返回，因此这个 goroutine 有退出路径。
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
