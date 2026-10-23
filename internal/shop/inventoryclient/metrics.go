// 本文件：从现有熔断器读取状态，并记录真正发送的查询重试。
//
// app 在创建客户端后、接收流量前调用 RegisterMetrics。采集时读取当前状态，
// 而不是只在状态变化时写 Gauge，这样等待期结束后的半开状态也不会一直显示为打开。
//
// 主要内容：
//   - RegisterMetrics：为两个熔断器登记只读采集函数。
//   - breakerValue：把库的状态名转换成计划书规定的 0、1、2。
//
// 阅读建议：状态值是项目契约，不能直接使用第三方库的枚举整数。

package inventoryclient

import (
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/resilience"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"github.com/prometheus/client_golang/prometheus"
)

// RegisterMetrics 必须在客户端开始使用之前调用，之后 retryCounter 只负责线程安全地加数。
func (client *Client) RegisterMetrics(metrics *telemetry.Metrics) error {
	client.retryCounter = metrics.Retries.WithLabelValues("inventory-service", "GetStock")
	for method, breaker := range map[string]*resilience.Breaker{"GetStock": client.getBreaker, "Deduct": client.deductBreaker} {
		gauge := prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "circuit_breaker_state", Help: "熔断状态：0 关闭，1 半开，2 打开。",
			ConstLabels: prometheus.Labels{"target": "inventory-service", "method": method}}, func() float64 { return breakerValue(breaker.State()) })
		if err := metrics.Register(gauge); err != nil {
			return err // 同一客户端重复注册属于装配错误，应在启动时报告。
		}
	}
	return nil
}

// breakerValue 显式列出三种状态，读者可以直接与告警表达式对照。
func breakerValue(state string) float64 {
	switch state {
	case "open":
		return 2
	case "half-open":
		return 1
	default:
		return 0
	}
}
