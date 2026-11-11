// 本文件：服务通过 Notify 接收退出通知，再由 Shutdown 按组装层安排的顺序关闭资源。
// 从 Shutdown 读起，可以看到已经收到退出信号的服务怎样给清理工作留出时间。

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Hook 由服务组装层提供，声明停机时应关闭的资源。
type Hook struct {
	Name  string                      // 会出现在关闭错误中，避免包含凭据。
	Close func(context.Context) error // 必须响应清理期限，Shutdown 会等待此回调返回。
}

// Notify 在收到 Ctrl+C、SIGTERM 或父上下文取消时通知服务退出。调用方退出时须调用 stop 注销信号监听。
func Notify(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}

// Shutdown 按传入顺序关闭资源，所有回调共用一个期限。调用方须避免并发关闭同一资源。
func Shutdown(parent context.Context, timeout time.Duration, hooks ...Hook) error {
	if timeout <= 0 {
		return fmt.Errorf("优雅停机超时必须大于 0")
	}
	// 停机通常由父上下文取消触发。清理仍需要其中的日志字段，但必须另设期限，才能给正在处理的请求留出收尾时间。
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), timeout)
	defer cancel()
	var failures []error
	// 一个资源关闭失败也要继续关闭后续资源，最后一起报告各项错误。
	for _, hook := range hooks {
		if hook.Close == nil {
			failures = append(failures, fmt.Errorf("关闭 %s：未提供关闭函数", hook.Name))
			continue
		}
		if err := hook.Close(ctx); err != nil {
			failures = append(failures, fmt.Errorf("关闭 %s：%w", hook.Name, err))
		}
	}
	// 回调可能在截止时间过后返回成功。总预算用尽也要让调用方知道。
	if err := ctx.Err(); err != nil {
		failures = append(failures, fmt.Errorf("优雅停机超过总预算：%w", err))
	}
	return errors.Join(failures...)
}
