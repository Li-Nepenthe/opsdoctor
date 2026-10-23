// 本文件：从实际完成的 HTTP 尝试计算吞吐、错误率和延迟。
//
// 调度目标不等于实际 QPS。这里按完成次数除以真实经过时间计算，重试也算一次 HTTP 尝试。
// 延迟按一毫秒分桶保存，内存大小固定。P50、P99 是桶上界估计，不冒充纳秒精确值。
// P99 表示把请求按耗时排好后，百分之九十九的请求不超过的那个位置。P50 则看中间位置。
//
// 主要内容：
//   - histogram：保存完成数、错误数、请求类型和延迟桶。
//   - statistics：同时保存十秒窗口与整次运行的统计。
//   - record、drop：记录完成请求或调度未能发送的请求。
//   - snapshot、percentile：生成报表，并按需要清空窗口。
//
// 阅读建议：ErrorRate 的分母是已完成尝试，Dropped 单独列出，不能隐藏调度压力。

package loadgen

import (
	"sync"
	"time"
)

// maximumLatencyMS 覆盖允许的三十秒请求超时并留一倍余量，极端超出值落入最后一桶并计数。
const maximumLatencyMS = 60000

// histogram 使用固定数组，持续压测不会无限保存每条请求的延迟样本。
type histogram struct {
	requests uint64                       // 实际完成的 HTTP 尝试数，包含重试。
	errors   uint64                       // 网络、解析或非成功响应的次数。
	dropped  uint64                       // 调度器因工作队列满而未能发送的次数。
	kinds    [3]uint64                    // 商品、下单、列表三种请求分别完成多少次。
	buckets  [maximumLatencyMS + 1]uint64 // 下标是向上取整的毫秒数。
	overflow uint64                       // 超过最大桶上界的真实样本数，报告时不能藏掉。
}

// statistics 的一把锁保护两个直方图，窗口和总计看到同一次记录。
type statistics struct {
	mu            sync.Mutex // 发送工作者并发调用 record，报告循环调用 snapshot。
	started       time.Time  // 整次运行的起点。
	windowStarted time.Time  // 上一份窗口报告生成的时间。
	total         histogram  // 全程累计，不因十秒报告清空。
	window        histogram  // 每次窗口报告后归零。
}

// report 是日志与控制接口使用的数值快照，不含内部锁或大数组。
type report struct {
	ElapsedSeconds  float64 `json:"elapsed_seconds"`  // 实际统计窗口时长。
	Requests        uint64  `json:"requests"`         // 实际完成数。
	Errors          uint64  `json:"errors"`           // 实际失败数。
	Dropped         uint64  `json:"dropped"`          // 没发出去的调度数，独立报告。
	QPS             float64 `json:"qps"`              // Requests / ElapsedSeconds。
	ErrorRate       float64 `json:"error_rate"`       // 0..1 的比例。
	P50MS           int     `json:"p50_ms"`           // 一毫秒桶的百分之五十上界。
	P99MS           int     `json:"p99_ms"`           // 一毫秒桶的百分之九十九上界。
	ProductRequests uint64  `json:"product_requests"` // 包含所有实际商品查询尝试。
	OrderRequests   uint64  `json:"order_requests"`   // 包含下单的超时重试。
	ListRequests    uint64  `json:"list_requests"`    // 实际订单列表查询。
	LatencyOverflow uint64  `json:"latency_overflow"` // 不为零时最大桶不能代表真实尾延迟。
}

// newStatistics 在开始调度前创建，不把配置和连接准备时间算进压测速率。
func newStatistics(now time.Time) *statistics { return &statistics{started: now, windowStarted: now} }

// record 向上取整延迟，百分位不会因取整而比该桶内的真实请求更快。
func (stats *statistics) record(kind requestKind, duration time.Duration, failed bool) {
	stats.mu.Lock()
	defer stats.mu.Unlock()
	index := int((duration + time.Millisecond - 1) / time.Millisecond)
	if index < 0 {
		index = 0
	}
	// 一次调用同时更新窗口与总计，不能让两个报告遗漏不同的请求。
	for _, data := range []*histogram{&stats.window, &stats.total} {
		data.requests++
		data.kinds[kind]++
		if failed {
			data.errors++
		}
		if index > maximumLatencyMS {
			data.overflow++
			data.buckets[maximumLatencyMS]++
		} else {
			data.buckets[index]++
		}
	}
}

// drop 记录没有真正发出的计划请求，既不冒充完成数，也不把它隐去。
func (stats *statistics) drop() {
	stats.mu.Lock()
	defer stats.mu.Unlock()
	stats.window.dropped++
	stats.total.dropped++
}

// snapshot 在持锁时复制小报表。resetWindow 只清空十秒窗口，最终总计始终保留。
func (stats *statistics) snapshot(now time.Time, resetWindow bool) report {
	// 报告和清空必须在同一把锁内，否则刚完成的请求可能被漏算。
	stats.mu.Lock()
	defer stats.mu.Unlock()
	data, started := &stats.total, stats.started
	if resetWindow {
		data, started = &stats.window, stats.windowStarted
	}
	result := report{ElapsedSeconds: now.Sub(started).Seconds(), Requests: data.requests, Errors: data.errors, Dropped: data.dropped,
		P50MS: percentile(data, 50), P99MS: percentile(data, 99), ProductRequests: data.kinds[productRequest], OrderRequests: data.kinds[orderRequest], ListRequests: data.kinds[listRequest], LatencyOverflow: data.overflow}
	if result.ElapsedSeconds > 0 {
		result.QPS = float64(data.requests) / result.ElapsedSeconds
	}
	if data.requests > 0 {
		result.ErrorRate = float64(data.errors) / float64(data.requests)
	}
	if resetWindow {
		stats.window = histogram{}
		stats.windowStarted = now
	}
	return result
}

// percentile 使用向上取整的名次，再累加桶计数，找到包含该名次的桶。
func percentile(data *histogram, percent uint64) int {
	if data.requests == 0 {
		return 0
	}
	target := (data.requests*percent + 99) / 100
	var count uint64
	for index, value := range data.buckets {
		count += value
		if count >= target {
			return index
		}
	}
	return maximumLatencyMS
}
