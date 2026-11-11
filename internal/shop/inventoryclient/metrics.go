// 本文件：采集时读取查询与扣减熔断器的当前状态，让等待期结束后的半开状态也能及时反映到指标上。
// RegisterMetrics 在订单服务接流量前注册指标，查询路径只在再次实际发送时增加重试计数。

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

// breakerValue 将状态映射为告警使用的固定数值。
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
