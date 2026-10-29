// 本文件：为一次诊断固定工具菜单和默认查询起点。
//
// 一场故障可能已经持续了几分钟。每轮只查“最近五分钟”会把故障刚开始时的证据漏掉。
// 这里从故障开始前五分钟一直查到当前时刻。模型明确填写回看秒数时，以它的参数为准。
//
// 主要内容：ForDiagnosis 复制菜单并绑定起点；diagnosisWindow 在调用时补上默认窗口。
// 阅读建议：副本只修改自己的参数说明，不会影响另一个系统或同时运行的诊断。

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

// ForDiagnosis 先筛选菜单，再给有时间范围的工具绑定起点。nil 表示不筛选，空 map 表示没有工具。
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
