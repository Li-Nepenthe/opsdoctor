// 本文件：给可以重复执行的调用加上有上限的重试。
//
// inventoryclient 只把查库存交给这里，扣库存不会经过这里。
// 重试与业务分开后，测试可以直接检查等待时间，不必真的断开网络。
//
// 主要内容：
//   - RetryPolicy：说明最多重试几次，以及两次尝试之间等多久。
//   - NewRetrier：检查配置并准备定时等待。
//   - Do：失败后先判断是否值得再试，再检查剩余时间。
//   - waitContext：等待退避时间，取消时提前结束。
//
// 阅读建议：先读 Do 的三个退出条件，再看 delay 怎样加上随机时间。

package resilience

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"
)

// RetryPolicy 把重试限制写在一起，避免每个调用各自决定什么时候再试。
type RetryPolicy struct {
	MaxRetries int           // 首次调用之外的尝试次数，0 表示不重试，最多 10。
	Backoff    time.Duration // 第一次等待的基础时长，后面逐次翻倍；0 表示不等待。
	MinBudget  time.Duration // 除去等待后，至少还要给下一次调用留下这么长时间。
}

// Retrier 保存固定策略。wait 和 jitter 由构造函数设置，测试可以换成可预测的实现。
type Retrier struct {
	policy RetryPolicy                                // policy 构造后不再修改。
	wait   func(context.Context, time.Duration) error // wait 使用 context 响应取消。
	jitter func(time.Duration) time.Duration          // jitter 返回小于基础等待时间的随机增量。
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
		// 左移一位相当于乘二。最多十次，构造时也限制了基础时长，不会超出 Duration 范围。
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

// waitContext 不使用 Sleep，因为调用方取消时，不应该继续睡到退避时间结束。
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
