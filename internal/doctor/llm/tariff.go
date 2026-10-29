// 本文件：根据请求发出时的北京时间选择 DeepSeek 的峰时或闲时价格。
//
// 长批次可能跨过两个时段。只在启动时读一次单价，会把后半批的优惠遗漏。
// 日期例外显式来自配置，不靠程序猜中国法定节假日；每次请求把生效单价记到账本。
//
// 主要内容：DeepSeekTariff 校验日期并创建价格选择函数。
// 阅读建议：星期和小时只决定折扣，不改变模型、账户或调用顺序。

package llm

import (
	"fmt"
	"time"
)

// DeepSeekTariff 按官方北京时间工作日 9–12、14–18 点峰时规则，其他时间单价减半。
func DeepSeekTariff(peak Prices, holidays []string) (func(time.Time) Prices, error) {
	if peak.InputMicros <= 0 || peak.CacheHitMicros <= 0 || peak.OutputMicros <= 0 || peak.CacheWriteMicros != 0 || peak.InputMicros%2 != 0 || peak.CacheHitMicros%2 != 0 || peak.OutputMicros%2 != 0 {
		return nil, fmt.Errorf("DeepSeek 峰时单价必须能精确折半，且不含缓存写入价")
	}
	// 日期集合复制到闭包里，调用方之后改切片不会改变冻结的价格安排。
	dates := map[string]bool{}
	for _, date := range holidays {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return nil, fmt.Errorf("DeepSeek 节假日须为 YYYY-MM-DD")
		}
		dates[date] = true
	}
	beijing := time.FixedZone("UTC+8", 8*60*60)
	return func(now time.Time) Prices {
		local := now.In(beijing)
		hour := local.Hour()
		weekday := local.Weekday() != time.Saturday && local.Weekday() != time.Sunday
		if weekday && !dates[local.Format("2006-01-02")] && (hour >= 9 && hour < 12 || hour >= 14 && hour < 18) {
			return peak
		}
		return Prices{InputMicros: peak.InputMicros / 2, CacheHitMicros: peak.CacheHitMicros / 2, OutputMicros: peak.OutputMicros / 2}
	}, nil
}
