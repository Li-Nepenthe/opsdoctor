// 本文件：不用真实等待或修改系统时间，验证分级校时规则的每个边界。
//
// 主要内容：TestPolicy 检查单次阈值与记录；TestRollingWindow 检查十分钟边界；
// TestMonitorAcrossSegments 检查独立采样段共享历史。
// 阅读建议：一纳秒外也要准确判断，不能把秒数取整后再比较。

package clockcheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// TestPolicy 同时验证是否暂停和是否留档；允许继续不等于忽略发生过的修正。
func TestPolicy(t *testing.T) {
	for _, test := range []struct {
		name             string
		delta            time.Duration
		recorded, paused bool
	}{
		{"正记录边界", 100 * time.Millisecond, false, false}, // 恰好100ms不记录。
		{"负记录边界", -100 * time.Millisecond, false, false},
		{"正边界外", 100*time.Millisecond + 1, true, false}, // 只写证据，不暂停。
		{"负边界外", -100*time.Millisecond - 1, true, false},
		{"已发生918ms", 917938571, true, false}, // 原失败保留，新规则下只记录。
		{"回拨一秒", -time.Second, true, false},
		{"回拨一秒外", -time.Second - 1, true, true},
		{"前跳五秒", 5 * time.Second, true, false},
		{"前跳五秒外", 5*time.Second + 1, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var events []Event
			monitor := NewMonitor(func(e Event) error { events = append(events, e); return nil })
			at := time.Now()
			err := monitor.observe(at, at, test.delta)
			if (err != nil) != test.paused || (len(events) > 0) != test.recorded {
				t.Fatalf("events=%+v err=%v", events, err)
			}
			if len(events) > 0 && (events[0].DeltaNS != int64(test.delta) || events[0].Paused != test.paused || (events[0].Direction == "backward") != (test.delta < 0)) {
				t.Fatal(events)
			}
		})
	}
	monitor := NewMonitor(func(Event) error { return errors.New("disk full") })
	if monitor.observe(time.Now(), time.Now(), 200*time.Millisecond) == nil {
		t.Fatal("日志丢失却继续")
	}
}

// TestMonitorAcrossSegments 模拟四段独立采样；每段 elapsed 从零开始，但计数不能重置。
func TestMonitorAcrossSegments(t *testing.T) {
	monitor := NewMonitor(nil)
	for segment := range 4 {
		var input bytes.Buffer
		encoder := json.NewEncoder(&input)
		for sample := range 61 {
			wall := int64(1793277600000000000) + int64(sample)*int64(time.Second)
			if sample > 30 {
				wall += int64(600 * time.Millisecond)
			} // 每段恰好一次中等修正。
			if err := encoder.Encode(reading{Wall: wall, Elapsed: int64(sample) * int64(time.Second)}); err != nil {
				t.Fatal(err)
			}
		}
		if err := monitor.Check(&input); (err != nil) != (segment == 3) {
			t.Fatalf("段=%d err=%v", segment, err)
		}
	}
}

// TestRollingWindow 在不同分钟放入四次跳变；日期乱跳也不能改变历史的淘汰时刻。
func TestRollingWindow(t *testing.T) {
	start := time.Now()
	for _, gap := range []time.Duration{10 * time.Minute, 10*time.Minute + 1} {
		monitor := NewMonitor(nil)
		for i := range 3 {
			if err := monitor.observe(start.Add(time.Duration(i)*time.Minute), start.Add(-time.Duration(i)*time.Hour), 600*time.Millisecond); err != nil {
				t.Fatal(err)
			}
		}
		err := monitor.observe(start.Add(gap), start, -600*time.Millisecond)
		if (err != nil) != (gap == 10*time.Minute) {
			t.Fatalf("gap=%s err=%v", gap, err)
		}
	}
	monitor := NewMonitor(nil)
	for i := range 4 { // 恰好0.5秒不属于“超过0.5秒”。
		if err := monitor.observe(start.Add(time.Duration(i)*time.Minute), start, 500*time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
	if len(monitor.recent) != 0 {
		t.Fatal("误计边界")
	}
}
