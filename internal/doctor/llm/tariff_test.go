// 本文件：固定日期测试闲时价格，不依赖测试机器所在时区。
//
// 主要内容：TestDeepSeekTariff 覆盖峰时边缘、午间、周末和配置的节假日。
// 阅读建议：用 UTC 输入验证转换，避免把本机西海岸时间误当北京时间。

package llm

import (
	"testing"
	"time"
)

// TestDeepSeekTariff 同一组峰时价格在各边界必须准确切换，不能改变缓存比例。
func TestDeepSeekTariff(t *testing.T) {
	peak := Prices{InputMicros: 2000000, CacheHitMicros: 40000, OutputMicros: 8000000}
	schedule, err := DeepSeekTariff(peak, []string{"2026-10-01"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		text string
		busy bool
	}{
		{"2026-10-29T00:59:59Z", false}, // 北京时间早上九点前。
		{"2026-10-29T01:00:00Z", true},  // 九点整进入峰时。
		{"2026-10-29T04:00:00Z", false}, // 中午十二点进入闲时。
		{"2026-10-29T06:00:00Z", true},  // 下午两点重新进入峰时。
		{"2026-10-29T10:00:00Z", false}, // 下午六点结束峰时。
		{"2026-10-31T01:00:00Z", false}, // 周末全天闲时。
		{"2026-10-01T01:00:00Z", false}, // 显式配置的节假日覆盖工作日。
	} {
		now, _ := time.Parse(time.RFC3339, test.text)
		want := peak
		if !test.busy {
			want.InputMicros /= 2
			want.CacheHitMicros /= 2
			want.OutputMicros /= 2
		}
		if got := schedule(now); got != want {
			t.Fatalf("%s 单价=%+v", test.text, got)
		}
	}
	if _, err := DeepSeekTariff(peak, []string{"tomorrow"}); err == nil {
		t.Fatal("接受含糊节假日")
	}
}
