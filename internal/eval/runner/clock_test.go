// 本文件：用固定样本证明时钟跳变会拦住评测，不在单元测试里启动 Docker。
//
// 主要内容：TestClockSamples 检查完整一分钟和跳变；TestClockGuard 检查取消原因传递。
// 阅读建议：样本包含真实预检出现过的 0、+12、-10 秒步长，不放宽到“差不多一秒”。

package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestClockSamples 前六十次正常才能通过，少样、重复和跳变都必须阻止开跑。
func TestClockSamples(t *testing.T) {
	for _, test := range []struct {
		name  string
		step  int64
		lines int
		valid bool
	}{
		{"完整一分钟", 1, 61, true},   // 初始读数加六十个一秒后的读数。
		{"时钟停住", 0, 61, false},   // 本机实测第 53 次出现过零步长。
		{"向前跳变", 12, 61, false},  // 本机实测第 54 次快进十二秒。
		{"向后跳变", -10, 61, false}, // 本机实测第 55 次倒退十秒。
		{"少一个样本", 1, 60, false},  // Docker 提前退出不能算通过。
		{"多一个样本", 1, 62, false},  // 输出必须符合固定协议。
	} {
		t.Run(test.name, func(t *testing.T) {
			var data strings.Builder
			current := int64(1793267900)
			for i := range test.lines {
				if i > 0 {
					if i == 53 {
						current += test.step
					} else {
						current++
					}
				}
				fmt.Fprintln(&data, current)
			}
			if err := clockSamples(strings.NewReader(data.String())); (err == nil) != test.valid {
				t.Fatalf("valid=%v err=%v", test.valid, err)
			}
		})
	}
	if err := clockSamples(strings.NewReader("无法连接 Docker\n")); err == nil {
		t.Fatal("把错误文字当成时钟")
	}
}

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
