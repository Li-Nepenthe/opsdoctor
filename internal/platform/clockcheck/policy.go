// 本文件：把小幅校时、大幅跳变和反复跳变分开处理。
//
// 主要内容：Monitor 保留十分钟历史；Event 记录时间、方向和大小；
// observe 实施用户批准的门槛，NewMonitor 为连续采样建立独立状态。
// 阅读建议：历史淘汰用单调时间，不能用正在被检查的日期来决定十分钟是否过去。

package clockcheck

import (
	"fmt"
	"time"
)

// Event 是一条可写入台账的事实。DeltaNS 为负表示回拨，正表示前跳。
type Event struct {
	Time      time.Time `json:"time_utc"`          // 容器样本日期，仅作事件定位。
	Direction string    `json:"direction"`         // forward 或 backward。
	DeltaNS   int64     `json:"delta_ns"`          // 日期增量减去单调增量。
	Recent    int       `json:"over_500ms_in_10m"` // 本次在内的十分钟内大修正次数。
	Paused    bool      `json:"paused"`            // 小修正只留档，不能伪装成实验失败。
}

// Monitor 由一个采样循环持有。不要每分钟重建，否则反复跳变会被清零。
type Monitor struct {
	recent []time.Time       // 宿主机单调时间；不跨进程冒充可恢复的计时。
	record func(Event) error // 写失败也暂停，不能继续却丢掉用户要求的记录。
}

// NewMonitor 允许单元测试直接捕获事件；真实运行传入持久化日志出口。
func NewMonitor(record func(Event) error) *Monitor { return &Monitor{record: record} }

// observe 使用严格“大于”：恰好回拨一秒、前跳五秒或三次修正都没有越界。
func (monitor *Monitor) observe(at, wall time.Time, delta time.Duration) error {
	if delta >= -100*time.Millisecond && delta <= 100*time.Millisecond {
		return nil // 微小校时不额外写事件，但原始采样仍可单独导出。
	}
	kept := monitor.recent[:0]
	for _, previous := range monitor.recent {
		if at.Sub(previous) <= 10*time.Minute {
			kept = append(kept, previous) // 恰在十分钟边界上的事件仍计入，避免少算。
		}
	}
	monitor.recent = kept
	if delta < -500*time.Millisecond || delta > 500*time.Millisecond {
		monitor.recent = append(monitor.recent, at)
	}
	paused := delta < -time.Second || delta > 5*time.Second || len(monitor.recent) > 3
	direction := "forward"
	if delta < 0 {
		direction = "backward"
	}
	event := Event{Time: wall.UTC(), Direction: direction, DeltaNS: int64(delta), Recent: len(monitor.recent), Paused: paused}
	if monitor.record != nil {
		if err := monitor.record(event); err != nil {
			return fmt.Errorf("无法保存校时事件，评测暂停：%w", err)
		}
	}
	if paused {
		return fmt.Errorf("时钟修正 %s，十分钟内超过 0.5 秒共 %d 次；超过回拨 1 秒、前跳 5 秒或第四次修正门槛，评测暂停", delta, len(monitor.recent))
	}
	return nil
}
