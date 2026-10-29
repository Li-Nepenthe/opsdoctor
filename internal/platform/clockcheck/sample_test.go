// 本文件：用固定读数验证时钟守卫，不在单元测试里修改系统日期。
//
// 主要内容：TestCheck 覆盖跳变和边界；TestSample 用虚拟时间验证采样与取消。
// 阅读建议：调度晚了，两种增量一起变大；校时则只改变日期一侧。

package clockcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// TestCheck 在第 53 次放入变化，复现原实测位置，仍要求全部六十次采样。
func TestCheck(t *testing.T) {
	for _, test := range []struct {
		name         string
		shift, delay time.Duration
		lines        int
		valid        bool
	}{
		{"正常", 0, 0, 61, true},                              // 初始读数加六十个步长。
		{"采样晚了一秒", 0, time.Second, 61, true},                // 两边都晚，不能误报时钟跳变。
		{"实测小幅回调", -30 * time.Millisecond, 0, 61, true},     // 小幅校时在用户批准的范围内。
		{"正边界", 100 * time.Millisecond, 0, 61, true},        // 恰好一百毫秒不超过上限。
		{"负边界", -100 * time.Millisecond, 0, 61, true},       // 前后两个方向使用同一边界。
		{"正边界外一纳秒", 100*time.Millisecond + 1, 0, 61, true},  // 超过记录边界，但未超过暂停边界。
		{"负边界外一纳秒", -100*time.Millisecond - 1, 0, 61, true}, // 小幅回拨留档，不立即拦截。
		{"原零步长", -time.Second, 0, 61, true},                 // 恰好回拨一秒按新规则留档。
		{"原十二秒", 11 * time.Second, 0, 61, false},            // 原始大幅前跳仍然被拦住。
		{"原负十秒", -11 * time.Second, 0, 61, false},           // 原始大幅回拨仍然被拦住。
		{"漏样本", 0, 0, 60, false},                            // Docker 提前结束不算完成。
		{"多样本", 0, 0, 62, false},                            // 不接受另一个协议的输出。
		{"总时间超限", 0, 16 * time.Second, 61, false},           // 不是跳变也不能无限等待繁忙机器。
	} {
		t.Run(test.name, func(t *testing.T) {
			var data bytes.Buffer
			encoder := json.NewEncoder(&data)
			value := reading{Wall: 1793277600000000000}
			for index := range test.lines {
				if index > 0 {
					value.Wall += int64(time.Second)
					value.Elapsed += int64(time.Second)
				}
				if index == 53 {
					value.Wall += int64(test.shift + test.delay)
					value.Elapsed += int64(test.delay)
				}
				if err := encoder.Encode(value); err != nil {
					t.Fatal(err)
				}
			}
			if err := Check(&data); (err == nil) != test.valid {
				t.Fatalf("valid=%v err=%v", test.valid, err)
			}
		})
	}
	// 错误文字、重复单调读数和非法起点，不能靠刚好有六十一行蒙混过去。
	for _, data := range []string{"Docker failed\n", `{"wall_ns":1,"elapsed_ns":1}`, "{\"wall_ns\":1,\"elapsed_ns\":0}\n{\"wall_ns\":2,\"elapsed_ns\":0}\n"} {
		if err := Check(strings.NewReader(data)); err == nil {
			t.Fatal("接受了非法样本")
		}
	}
}

// TestSample 用 Go 虚拟时间走完一分钟，CI 不真的等待，也不访问 Docker。
func TestSample(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var data bytes.Buffer
		if err := Sample(t.Context(), &data); err != nil {
			t.Fatal(err)
		}
		if err := Check(&data); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := Sample(ctx, &data); err != context.Canceled {
			t.Fatal("取消没有停止采样", err)
		}
	})
}
