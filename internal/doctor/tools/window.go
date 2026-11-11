// 本文件：为一次诊断筛选独立的工具菜单，并设置默认查询时间范围。
// 故障可能已持续数分钟，默认窗口从故障前五分钟起算，最长一小时。调用者明确指定的回看范围优先。

package tools

import (
	"context"
	"fmt"
	"maps"
	"math"
	"time"
)

// EvidenceLookback 是故障前的对照时间；评测器也用它决定两场实验至少相隔多久。
const EvidenceLookback = 5 * time.Minute

// ForDiagnosis 先筛选菜单，再绑定时间起点。未传筛选表会保留全部工具，传入空表则全部移除。
// 返回的仍是 Registry，所有入口继续经过同一份校验、超时和截断。
func (registry *Registry) ForDiagnosis(start time.Time, allowed map[string]bool) *Registry {
	result := &Registry{tools: make(map[string]tool)}
	for name, item := range registry.tools {
		if allowed != nil && !allowed[name] {
			continue // 被实验组移除的工具连执行入口也不存在，不能只从提示词里隐藏。
		}
		item.definition.Parameters.Properties = maps.Clone(item.definition.Parameters.Properties)
		if property, ok := item.definition.Parameters.Properties["lookback_seconds"]; ok && !start.IsZero() {
			property.Default = nil // 留到调用时计算；诊断后半程的窗口应包含已经过去的时间。
			property.Description += "；省略时从故障开始前五分钟查询，最长一小时"
			item.definition.Parameters.Properties["lookback_seconds"] = property
			if name == "query_metrics" {
				step := item.definition.Parameters.Properties["step_seconds"]
				step.Default = nil // 更长的默认窗口需要相应加大步长，避免超过采样点上限。
				item.definition.Parameters.Properties["step_seconds"] = step
			}
			item.run = diagnosisWindow(item.run, name, start, time.Now)
			// 默认值已改变，副本需要自己的规则；否则旧校验器仍会补回固定窗口。
			item.parameters = item.definition.Parameters.resolve()
		}
		result.tools[name] = item
	}
	return result
}

// diagnosisWindow 只补缺省参数，显式的即时查询（0 秒）和更早的查询都不会被覆盖。
func diagnosisWindow(next func(context.Context, arguments) (output, error), name string, start time.Time, now func() time.Time) func(context.Context, arguments) (output, error) {
	return func(ctx context.Context, args arguments) (output, error) {
		// 参数存在与值为零是两回事；0 秒的即时查询必须保留。
		_, explicit := args["lookback_seconds"]
		if !explicit {
			seconds := int(math.Ceil(now().Sub(start.Add(-EvidenceLookback)).Seconds()))
			args["lookback_seconds"] = min(3600, max(300, seconds))
		}
		if name == "query_metrics" {
			if _, supplied := args["step_seconds"]; !supplied {
				args["step_seconds"] = max(15, (args.integer("lookback_seconds")+239)/240)
			}
		}
		value, err := next(ctx, args)
		if !explicit {
			value.summary += fmt.Sprintf("；默认回看 %d 秒（故障前五分钟至当前，最长一小时）", args.integer("lookback_seconds"))
		}
		return value, err
	}
}
