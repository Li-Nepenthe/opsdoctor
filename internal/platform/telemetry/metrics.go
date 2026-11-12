// 本文件：为每个进程单独注册 Prometheus 指标，统一采集服务、运行时和数据库连接池的数据。

package telemetry

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Version 由构建时的 -ldflags -X 写入，进程运行后不再修改。dev 表示直接 go run 的构建。
var Version = "dev"

// Metrics 的注册表归单个进程所有。订单号、商品 ID 等无限增长的值只进日志，不能用作标签。
type Metrics struct {
	Registry             *prometheus.Registry     // HTTP 端点只导出这一份注册表。
	registerer           prometheus.Registerer    // 自动为所有采集器补上同一个 service。
	HTTP                 *prometheus.HistogramVec // 按模板路由记录响应时间，单位秒。
	RPCServer            *prometheus.HistogramVec // 库存实际处理一次 RPC 的时间。
	RPCClient            *prometheus.HistogramVec // 一次真实 RPC 尝试的时间，不包含重试退避。
	Retries              *prometheus.CounterVec   // 只统计实际发出去的额外尝试。
	RateRejected         *prometheus.CounterVec
	Idempotency          *prometheus.CounterVec // new、replay、in_progress、needs_review 四类事件。
	CacheRequests        *prometheus.CounterVec // 缓存查询结果，包含防止重复回源时的再次检查。
	CacheLoads           *prometheus.CounterVec // 真正进入商品数据库查询的次数。
	CacheConcurrentLoads *prometheus.CounterVec // 开始查库时，同一商品已有查询在途的次数；只供诊断。
	Produce              *prometheus.CounterVec
	Messages             *prometheus.CounterVec   // 消费处理结果，重复消息单独统计。
	ProcessDuration      *prometheus.HistogramVec // 解析、业务处理与提交位点共用的耗时。
	Loadgen              *prometheus.CounterVec   // 压测器每次 HTTP 尝试，不把重试藏起来。
}

// NewMetrics 的名称都由本包固定，重复注册说明代码有错，所以用 MustRegister 尽早暴露。
func NewMetrics(service string) *Metrics {
	// 先登记通用运行时，再登记业务仪表。没有写入任何标签值的向量暂时不会导出样本。
	registry := prometheus.NewRegistry()
	metrics := &Metrics{Registry: registry, registerer: prometheus.WrapRegistererWith(prometheus.Labels{"service": service}, registry)}
	metrics.registerer.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	build := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "app_build_info", Help: "当前构建版本，值恒为 1。"}, []string{"version"})
	metrics.registerer.MustRegister(build)
	build.WithLabelValues(Version).Set(1)
	metrics.HTTP = metrics.Histogram("http_server_request_duration_seconds", "HTTP 处理耗时。", "method", "route", "status")
	metrics.RPCServer = metrics.Histogram("rpc_server_duration_seconds", "服务端 RPC 耗时。", "method", "code")
	metrics.RPCClient = metrics.Histogram("rpc_client_duration_seconds", "客户端每次 RPC 尝试耗时。", "target", "method", "code")
	metrics.Retries = metrics.Counter("rpc_client_retries_total", "实际发出的额外 RPC 尝试。", "target", "method")
	metrics.RateRejected = metrics.Counter("ratelimit_rejected_total", "被入口限流拒绝的次数。", "route")
	metrics.Idempotency = metrics.Counter("idempotency_requests_total", "幂等请求及待核对状态转换。", "result")
	metrics.CacheRequests = metrics.Counter("cache_requests_total", "缓存查询次数。", "cache", "result")
	metrics.CacheLoads = metrics.Counter("cache_loads_total", "商品查询真正回源的次数。", "cache")
	metrics.CacheConcurrentLoads = metrics.Counter("cache_concurrent_loads_total", "查库时同一商品已有查询在途的次数，仅作诊断证据。", "cache")
	metrics.Produce = metrics.Counter("kafka_produce_total", "消息发送确认结果。", "topic", "result")
	metrics.Messages = metrics.Counter("worker_messages_total", "消息处理结果。", "topic", "result")
	metrics.ProcessDuration = metrics.Histogram("worker_process_duration_seconds", "单条消息处理与提交位点的耗时。", "topic")
	metrics.Loadgen = metrics.Counter("loadgen_requests_total", "压测 HTTP 尝试次数。", "operation", "result")
	return metrics
}

// Counter 的标签名由代码固定；调用处还要保证标签值来自有限的分类，不能塞入用户输入。
func (metrics *Metrics) Counter(name, help string, labels ...string) *prometheus.CounterVec {
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
	metrics.registerer.MustRegister(counter)
	return counter
}

// Histogram 用从毫秒到十秒的桶覆盖正常请求和超时，特别保留告警门槛 1 秒。
func (metrics *Metrics) Histogram(name, help string, labels ...string) *prometheus.HistogramVec {
	histogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: name, Help: help,
		Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2, 5, 10}}, labels)
	metrics.registerer.MustRegister(histogram)
	return histogram
}

// Register 允许 app 登记熔断状态等按需读取的采集器，仍统一添加服务标签。
func (metrics *Metrics) Register(collector prometheus.Collector) error {
	return metrics.registerer.Register(collector)
}

// Database 读取 database/sql 已有的池统计，不另外发 SQL，也不会占用一个数据库连接。
func (metrics *Metrics) Database(database *sql.DB, name string) error {
	return metrics.Register(collectors.NewDBStatsCollector(database, name))
}
