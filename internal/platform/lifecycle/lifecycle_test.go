package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestShutdownOrderAndCancellation 验证正常工作取消后仍有清理时间，并且失败不会跳过后续关闭。
func TestShutdownOrderAndCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	var order []string
	failure := errors.New("模拟关闭失败")
	err := Shutdown(parent, time.Second,
		Hook{Name: "http", Close: func(ctx context.Context) error {
			if ctx.Err() != nil {
				t.Fatal("清理继承了已取消状态")
			}
			order = append(order, "http")
			return failure
		}},
		Hook{Name: "database", Close: func(context.Context) error {
			order = append(order, "database")
			return nil
		}},
	)
	// 合并错误仍保留 errors.Is 能力，调用方可以区分具体关闭失败的原因。
	if !errors.Is(err, failure) || strings.Join(order, ",") != "http,database" {
		t.Fatalf("顺序或错误丢失：%v，%v", order, err)
	}
}

// TestShutdownDeadline 用响应取消的假服务验证总预算，不创建真实服务器或后台线程。
func TestShutdownDeadline(t *testing.T) {
	// 10ms 足以触发预算路径；测试依赖 Done 信号而不是依赖操作系统精确计时。
	const budget = 10 * time.Millisecond
	err := Shutdown(context.Background(), budget, Hook{Name: "blocked", Close: func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("未传播超时：%v", err)
	}
}

// TestNotifyParentCancel 验证无需向测试进程发系统信号也能停止监听，避免影响并行测试。
func TestNotifyParentCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	ctx, stop := Notify(parent)
	defer stop()
	cancel()
	// 一秒只是测试的失败上限，不是服务运行时的关闭预算。
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("父上下文取消未传播")
	}
}
