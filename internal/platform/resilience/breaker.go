// 本文件：下游连续不健康时，暂停继续向它发送请求。
//
// inventoryclient 为查询和扣减各建一个 Breaker。一个接口失败，不直接影响另一个接口的额度。
// 底层状态变化交给 gobreaker，本文件只设置本项目的判断规则和日志。
// 关闭时正常放行，打开时直接拒绝。等一会儿进入半开，只放少量请求试探下游是否恢复。
//
// 主要内容：
//   - BreakerConfig：失败率门槛、观察窗口和恢复试探数量。
//   - NewBreaker：创建熔断器，并安装状态变化日志。
//   - Execute：允许通过时执行回调，打开时直接返回错误。
//   - State：供测试确认打开、半开和关闭的过程。
//
// 阅读建议：先看 ReadyToTrip。请求数太少时不急着判断，达到样本数后才比较失败比例。

package resilience

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/sony/gobreaker/v2"
)

// BreakerConfig 描述何时暂停请求，以及多久后允许少量请求探路。
type BreakerConfig struct {
	Enabled        bool          // false 时直接调用，用于对比有无保护的效果。
	MinRequests    uint32        // 至少观察多少次调用后才判断失败率。
	FailurePercent uint32        // 失败比例达到这个百分比时打开，取值 1..100。
	Interval       time.Duration // 关闭状态下多久清空一次计数。
	OpenTimeout    time.Duration // 打开后等待多久进入半开状态。
	HalfOpenMax    uint32        // 半开时允许的试探数，成功后恢复正常。
}

// Breaker 隐藏第三方的结果泛型。这里不保存业务返回值，只需要知道操作是否失败。
type Breaker struct {
	enabled bool                                // 是否启用保护，构造后不变。
	inner   *gobreaker.CircuitBreaker[struct{}] // 空结构体表示回调没有单独的数据返回值。
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

// Execute 在关闭或半开时调用 operation。回调里的错误也交回调用方，不替业务决定响应。
func (breaker *Breaker) Execute(operation func() error) error {
	if !breaker.enabled {
		return operation()
	}
	_, err := breaker.inner.Execute(func() (struct{}, error) {
		return struct{}{}, operation()
	})
	return err
}

// State 读取底层状态，测试用它检查恢复过程，而不是猜测内部计数。
func (breaker *Breaker) State() string {
	if !breaker.enabled {
		return "closed" // 禁用时不会拒绝流量，不能把旧的内部状态显示成仍在保护。
	}
	return breaker.inner.State().String()
}
