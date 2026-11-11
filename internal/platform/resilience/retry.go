// 本文件：只在调用可以重复执行、错误可恢复且剩余时间足够时重试。库存客户端只对查询使用这份策略。
// Do 用最近一次调用的结果判断是否继续，并在退避前为下一次调用预留执行时间。

package resilience

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"
)

// RetryPolicy 限制额外的尝试次数与退避时间。
type RetryPolicy struct {
	MaxRetries int           // 首次调用之外的尝试次数，0 表示不重试，最多 10。
	Backoff    time.Duration // 第一次等待的基础时长，后面逐次翻倍；0 表示不等待。
	MinBudget  time.Duration // 除去等待后，至少还要给下一次调用留下这么长时间。
}

// Retrier 按固定策略退避，等待可以随请求一起取消。
type Retrier struct {
	policy RetryPolicy
	wait   func(context.Context, time.Duration) error
	jitter func(time.Duration) time.Duration
}

// maximumRetries 防止配置把重试变成无限循环，也避免计算指数等待时间时溢出。
const maximumRetries = 10

// NewRetrier 在启动时拒绝不合理的策略，不把配置问题留到第一次请求。
func NewRetrier(policy RetryPolicy) (*Retrier, error) {
	if policy.MaxRetries < 0 || policy.MaxRetries > maximumRetries || policy.Backoff < 0 || policy.Backoff > time.Second || policy.MinBudget <= 0 {
		return nil, fmt.Errorf("重试配置不合法：次数须为 0..10，退避须为 0..1s，剩余预算须为正")
	}
	return &Retrier{policy: policy, wait: waitContext, jitter: func(base time.Duration) time.Duration {
		if base == 0 {
			return 0
		}
		// 随机打散下一次尝试，避免很多客户端失败后又在同一瞬间冲回来。
		return time.Duration(rand.Int64N(int64(base)))
	}}, nil
}

// Do 只根据调用方提供的 retryable 判断重试资格。业务错误通常应该立即返回。
func (retry *Retrier) Do(ctx context.Context, operation func() error, retryable func(error) bool) error {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := operation()
		if err == nil || attempt >= retry.policy.MaxRetries || !retryable(err) {
			return err
		}
		// 构造时已限制基础时长和重试次数，逐次翻倍不会溢出。
		base := retry.policy.Backoff * time.Duration(1<<attempt)
		delay := base + retry.jitter(base)
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < delay+retry.policy.MinBudget {
			// 只够等待、不够真正发请求时，再试没有意义，保留最后一次调用错误。
			return err
		}
		if err := retry.wait(ctx, delay); err != nil {
			return fmt.Errorf("等待下一次尝试：%w", err)
		}
	}
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
