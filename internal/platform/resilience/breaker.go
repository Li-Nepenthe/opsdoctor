// 本文件：按样本数和失败比例暂停下游请求，等待恢复后只放少量探测流量。
// 库存客户端通过 NewBreaker 为查询和扣减分别设置失败分类，状态切换由 ReadyToTrip 的门槛决定。

package resilience

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/sony/gobreaker/v2"
)

// BreakerConfig 描述何时暂停请求，以及多久后允许少量请求探路。
type BreakerConfig struct {
	Enabled        bool          // false 时放行全部请求，便于对照保护效果。
	MinRequests    uint32        // 至少观察多少次调用后才判断失败率。
	FailurePercent uint32        // 失败比例达到这个百分比时打开，取值 1..100。
	Interval       time.Duration // 关闭状态下多久清空一次计数。
	OpenTimeout    time.Duration // 打开后等待多久进入半开状态。
	HalfOpenMax    uint32        // 半开时允许的试探数，成功后恢复正常。
}

// Breaker 按调用方的错误分类统计健康状况。
type Breaker struct {
	enabled bool
	inner   *gobreaker.CircuitBreaker[struct{}]
}

// NewBreaker 由调用方说明哪些错误算下游故障，避免库存不足也把服务“熔断”。
func NewBreaker(name string, config BreakerConfig, logger *slog.Logger, successful, excluded func(error) bool) (*Breaker, error) {
	if config.MinRequests == 0 || config.FailurePercent == 0 || config.FailurePercent > 100 || config.Interval <= 0 || config.OpenTimeout <= 0 || config.HalfOpenMax == 0 {
		return nil, fmt.Errorf("熔断配置不合法：样本数、时间、试探数须为正，失败百分比须为 1..100")
	}
	settings := gobreaker.Settings{Name: name, MaxRequests: config.HalfOpenMax, Interval: config.Interval, Timeout: config.OpenTimeout,
		IsSuccessful: successful, IsExcluded: excluded,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// 先转 uint64 再乘，避免调用计数较大时百分比计算溢出。
			return counts.Requests >= config.MinRequests && uint64(counts.TotalFailures)*100 >= uint64(counts.Requests)*uint64(config.FailurePercent)
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			logger.Warn("下游调用熔断状态改变", "method", name, "from", from.String(), "to", to.String())
		},
	}
	return &Breaker{enabled: config.Enabled, inner: gobreaker.NewCircuitBreaker[struct{}](settings)}, nil
}

// Execute 返回下游的执行结果；熔断期间直接拒绝调用。
func (breaker *Breaker) Execute(operation func() error) error {
	if !breaker.enabled {
		return operation()
	}
	_, err := breaker.inner.Execute(func() (struct{}, error) {
		return struct{}{}, operation()
	})
	return err
}

// State 返回供观测使用的熔断状态。
func (breaker *Breaker) State() string {
	if !breaker.enabled {
		return "closed" // 禁用时不会拒绝流量，不能把旧的内部状态显示成仍在保护。
	}
	return breaker.inner.State().String()
}
