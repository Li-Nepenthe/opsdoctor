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

// Hook 表示一个关闭步骤；顺序由组装层确定，例如先 HTTP，再数据库。
type Hook struct {
	Name  string                      // Name 用于错误定位，不能包含凭据。
	Close func(context.Context) error // Close 必须响应 context 超时；返回 nil 表示关闭成功。
}

// Notify 在 Ctrl+C、SIGTERM 或父上下文取消时取消返回的 context。
// 它不等待退出信号；调用方必须 defer stop()，释放 signal.NotifyContext 内部的信号监听。
func Notify(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}

// Shutdown 按顺序执行 hooks，共用 timeout 这一总预算；返回所有关闭错误的合并结果。
// parent 可以已经取消；清理使用保留其值但去掉取消状态的新 context，否则服务会立即放弃排空请求。
// 函数同步等待 Close，不能替不响应 context 的实现强制终止；调用方不得并发关闭同一资源。
func Shutdown(parent context.Context, timeout time.Duration, hooks ...Hook) error {
	if timeout <= 0 {
		return fmt.Errorf("优雅停机超时必须大于 0")
	}
	// 第 1 步：建立独立的清理预算；defer 保证提前返回也释放定时器。
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), timeout)
	defer cancel()
	var failures []error
	// 第 2 步：一个资源关闭失败仍继续处理后面的资源，避免遗漏数据库连接等清理工作。
	for _, hook := range hooks {
		if hook.Close == nil {
			failures = append(failures, fmt.Errorf("关闭 %s：未提供关闭函数", hook.Name))
			continue
		}
		if err := hook.Close(ctx); err != nil {
			failures = append(failures, fmt.Errorf("关闭 %s：%w", hook.Name, err))
		}
	}
	if err := ctx.Err(); err != nil {
		failures = append(failures, fmt.Errorf("优雅停机超过总预算：%w", err))
	}
	return errors.Join(failures...)
}
