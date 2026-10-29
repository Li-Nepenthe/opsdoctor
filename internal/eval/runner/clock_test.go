// 本文件：用固定样本证明时钟跳变会拦住评测，不在单元测试里启动 Docker。
//
// 主要内容：TestClockGuard 检查取消原因传递。
// 阅读建议：具体采样和边界在 platform/clockcheck 测试，这里只验证执行器停止。

package runner

import (
	"context"
	"errors"
	"testing"
)

// TestClockGuard 保留时钟错误而不只返回泛泛的取消，同时结束自己的检查 goroutine。
func TestClockGuard(t *testing.T) {
	problem := errors.New("时钟跳变")
	ctx, stop := guardClock(t.Context(), func(context.Context) error { return problem })
	defer stop()
	<-ctx.Done()
	if !errors.Is(context.Cause(ctx), problem) {
		t.Fatal("丢失时钟暂停原因")
	}
	// 正常停止也要取消正在进行的检查，不能留下无人管理的本机 Docker 进程。
	ctx, stop = guardClock(t.Context(), func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
	stop()
	if !errors.Is(context.Cause(ctx), context.Canceled) {
		t.Fatal("停止没有传到采样器")
	}
}
