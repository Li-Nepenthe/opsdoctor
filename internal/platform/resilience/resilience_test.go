// 本文件：验证入口限流、重试边界和熔断恢复。
//
// 测试直接调用中间件或回调，不访问网络。这样失败只来自我们写的判断，而不是外部服务。
//
// 主要内容：
//   - TestRateLimit：桶里的票用完后，下一次请求应得到 429。
//   - TestRetryPolicy：检查次数、等待时间和剩余预算。
//   - TestRetryCancellation：等待期间取消，不再调用下游。
//   - TestBreakerRecovery：确认打开时不执行回调，试探成功后恢复。
//
// 阅读建议：先看每个表格用例旁的说明，再看假的 wait 如何记录等待时间。

package resilience

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sony/gobreaker/v2"
)

// TestRateLimit 用一个几乎不会在测试期间补满的桶，明确看到“放行一次，再拒绝一次”。
func TestRateLimit(t *testing.T) {
	middleware, err := RateLimit(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(middleware)
	router.GET("/", func(ctx *gin.Context) { ctx.Status(200) })
	// 第一次拿走唯一的票。紧接着再请求，就能检验拒绝路径。
	for _, want := range []int{200, 429} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
		if response.Code != want {
			t.Fatalf("状态=%d，想要 %d", response.Code, want)
		}
	}
	if _, err := RateLimit(0, 1); err == nil {
		t.Fatal("零速率未拒绝")
	}
}

// TestRetryPolicy 固定随机增量，让指数退避的每一步都可以直接核对。
func TestRetryPolicy(t *testing.T) {
	const backoff = 10 * time.Millisecond // 测试只记录这个时长，不真的睡眠。
	for _, testcase := range []struct {
		name      string          // name 说明哪条重试规则正在被验证。
		failures  int             // failures 表示前几次回调失败。
		retryable bool            // retryable 模拟暂时故障或业务拒绝。
		budget    time.Duration   // budget 用于测试时间不够时直接停下。
		calls     int             // calls 包含第一次尝试。
		waits     []time.Duration // waits 是期待的等待序列。
	}{
		{"立即成功", 0, true, time.Second, 1, nil},                                    // 成功不应该多发请求。
		{"两次后恢复", 2, true, time.Second, 3, []time.Duration{backoff, 2 * backoff}}, // 每次基础等待翻倍。
		{"达到上限", 9, true, time.Second, 3, []time.Duration{backoff, 2 * backoff}},  // 两次重试加首次，总共三次。
		{"业务错误", 9, false, time.Second, 1, nil},                                   // 业务拒绝不能靠重试解决。
		{"剩余预算不够", 9, true, time.Millisecond, 1, nil},                             // 不能把预算全部花在等下一次尝试上。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			retry, err := NewRetrier(RetryPolicy{MaxRetries: 2, Backoff: backoff, MinBudget: backoff})
			if err != nil {
				t.Fatal(err)
			}
			var waits []time.Duration
			retry.jitter = func(time.Duration) time.Duration { return 0 }
			retry.wait = func(_ context.Context, delay time.Duration) error { waits = append(waits, delay); return nil }
			ctx, cancel := context.WithTimeout(t.Context(), testcase.budget)
			defer cancel()
			calls := 0
			_ = retry.Do(ctx, func() error {
				calls++
				if calls <= testcase.failures {
					return errors.New("临时失败")
				}
				return nil
			}, func(error) bool { return testcase.retryable })
			if calls != testcase.calls || !slices.Equal(waits, testcase.waits) {
				t.Fatalf("次数=%d 等待=%v", calls, waits)
			}
		})
	}
}

// TestRetryCancellation 让第一次失败同时取消上下文，等待器必须立即结束。
func TestRetryCancellation(t *testing.T) {
	retry, err := NewRetrier(RetryPolicy{MaxRetries: 2, Backoff: time.Second, MinBudget: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	err = retry.Do(ctx, func() error { calls++; cancel(); return errors.New("失败") }, func(error) bool { return true })
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

// TestBreakerRecovery 用短窗口观察完整状态变化，确认打开期间没有偷偷执行下游回调。
func TestBreakerRecovery(t *testing.T) {
	const openTime = 20 * time.Millisecond // 单元测试只等二十毫秒，生产默认值由 app 配置。
	breaker, err := NewBreaker("test", BreakerConfig{Enabled: true, MinRequests: 2, FailurePercent: 50, Interval: time.Minute, OpenTimeout: openTime, HalfOpenMax: 1},
		slog.New(slog.NewTextHandler(io.Discard, nil)), func(err error) bool { return err == nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		_ = breaker.Execute(func() error { return errors.New("下游失败") })
	}
	called := false
	err = breaker.Execute(func() error { called = true; return nil })
	if !errors.Is(err, gobreaker.ErrOpenState) || called {
		t.Fatalf("打开后仍调用：%t %v", called, err)
	}
	time.Sleep(2 * openTime) // 跨过打开窗口，再用一次真实调用触发半开。
	err = breaker.Execute(func() error {
		if breaker.State() != "half-open" {
			t.Error("没有经过半开状态")
		}
		return nil
	})
	if err != nil || breaker.State() != "closed" {
		t.Fatalf("恢复失败：%v %s", err, breaker.State())
	}
}
