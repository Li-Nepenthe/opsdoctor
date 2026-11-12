// 本文件：登记指标、告警、消息积压和近期变更的查询入口。
// 范围查询要同时限制时间跨度和采样点数，变更工具只能从这些指标中报告实际观察到的变化。

package tools

import (
	"context"
	"fmt"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"time"
)

// addMetrics 指标查询先限制窗口和采样点，近期变更也只能从这些已采集指标推断。
func (registry *Registry) addMetrics(backend source.MetricsSource) {
	// 四种工具共用同一指标后端，消息积压也不绕过它直接连接 Kafka。
	registry.add("list_firing_alerts", "查看当前正式触发的告警；观察中的 pending 不在这里。", nil, nil, func(ctx context.Context, _ arguments) (output, error) {
		alerts, err := backend.Alerts(ctx)
		return output{data: alerts, summary: fmt.Sprintf("当前 %d 条 firing 告警", len(alerts))}, err
	})
	registry.add("query_metrics", "执行 PromQL。最多一小时、二十条序列、每条二百四十一个点；返回摘要。", map[string]Property{
		"query":            textProperty("PromQL 表达式，只在当前绑定的监控后端执行", 1, 2048),
		"lookback_seconds": integerProperty("回看秒数；0 表示即时查询", 0, 3600, 300),
		"step_seconds":     integerProperty("范围查询采样间隔，单位秒", 1, 300, 15),
	}, []string{"query"}, func(ctx context.Context, args arguments) (output, error) {
		window, step := args.integer("lookback_seconds"), args.integer("step_seconds")
		if window/step > 240 {
			return output{}, fmt.Errorf("%w：每条序列最多 241 个点，请增大 step_seconds", ErrInvalidArgument)
		}
		query := source.MetricQuery{Expression: args.text("query")}
		if window > 0 {
			query.End = time.Now().UTC()
			query.Start = query.End.Add(-time.Duration(window) * time.Second)
			query.Step = time.Duration(step) * time.Second
		}
		result, err := backend.Query(ctx, query)
		return summarizeMetrics(result), err
	})
	registry.add("kafka_consumer_lag", "通过指标查看消费者积压，不直接连接 Kafka 管理接口。", nil, nil, func(ctx context.Context, _ arguments) (output, error) {
		result, err := backend.Query(ctx, source.MetricQuery{Expression: "kafka_consumergroup_lag"})
		return summarizeMetrics(result), err
	})
	registry.add("recent_changes", "查看近期启动时间和版本标签。采样首次出现的时间不等于精确发布时间。", map[string]Property{"lookback_seconds": integerProperty("回看秒数", 60, 3600, 900)}, nil, func(ctx context.Context, args arguments) (output, error) {
		return recentChanges(ctx, backend, args.integer("lookback_seconds"))
	})
}
