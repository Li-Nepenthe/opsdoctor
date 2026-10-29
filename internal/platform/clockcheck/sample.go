// 本文件：用日期和单调时间的差值区分校时跳变与程序调度延迟。
//
// 主要内容：Sample 每秒输出一行；Check 校验六十个步长；reading 保存两种读数。
// 阅读建议：先看 now.Sub(start)，它保留 Go 的单调计时；UnixNano 则是墙上的日期。

package clockcheck

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

const count = 60                         // 一分钟连续样本；不能只检查一次日期。
const tolerance = 100 * time.Millisecond // 用户批准的跳变边界，允许实测的几十毫秒小幅校时。
const limit = 75 * time.Second           // 容器繁忙或采样不完整仍需暂停，不能无限等。

// reading 同一次 time.Now 同时提供两种读数，避免分开读取引入调度误差。
type reading struct {
	Wall    int64 `json:"wall_ns"`    // Unix 日期纳秒，用于观察校时。
	Elapsed int64 `json:"elapsed_ns"` // 从第一个样本起的单调纳秒，不受日期回拨影响。
}

// Sample 是纯本机检查，即使没有任何账号配置也能运行。
func Sample(parent context.Context, output io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, limit)
	defer cancel()
	encoder := json.NewEncoder(output)
	start := time.Now()
	now := start
	for index := 0; index <= count; index++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := encoder.Encode(reading{Wall: now.UnixNano(), Elapsed: int64(now.Sub(start))}); err != nil {
			return err
		}
		if index == count {
			return nil
		}
		// 不用固定日期安排下一次唤醒，否则日期回拨会影响采样器自己。
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			now = time.Now()
		}
	}
	return nil
}

// Check 流式读取，任何大于边界的前跳或回拨都立即返回，不等满一分钟。
func Check(input io.Reader) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 256), 256)
	var previous reading
	lines := 0
	for scanner.Scan() {
		var current reading
		if json.Unmarshal(scanner.Bytes(), &current) != nil || current.Wall <= 0 || current.Elapsed < 0 || current.Elapsed > int64(limit) || lines > count {
			return fmt.Errorf("容器时钟样本格式或数量不合法，评测暂停")
		}
		if lines == 0 {
			if current.Elapsed != 0 {
				return fmt.Errorf("容器时钟缺少初始读数，评测暂停")
			}
		} else {
			monotonic := time.Duration(current.Elapsed - previous.Elapsed)
			wall := time.Duration(current.Wall - previous.Wall)
			// 两个正日期相减不会溢出；单调值先限制在 75 秒内，边界比较也不会溢出。
			if monotonic < time.Second {
				return fmt.Errorf("容器时钟第 %d 次采样间隔不足一秒，评测暂停", lines)
			}
			if wall < monotonic-tolerance || wall > monotonic+tolerance {
				return fmt.Errorf("容器时钟第 %d 次日期增量 %s，单调增量 %s，差值超过 100ms；评测暂停，请检查校时", lines, wall, monotonic)
			}
		}
		previous = current
		lines++
	}
	if scanner.Err() != nil || lines != count+1 {
		return fmt.Errorf("容器时钟样本不完整，评测暂停")
	}
	return nil
}
