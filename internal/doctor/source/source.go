// 本文件：约定工具可向已绑定的数据源查询哪些证据，屏蔽各后端的协议和返回格式。

package source

import (
	"context"
	"time"
)

// MetricsSource 把“查指标”和“看告警”放在同一个系统绑定的数据源上。
type MetricsSource interface {
	Query(context.Context, MetricQuery) (MetricResult, error) // 时间范围由工具先检查，再交给适配器。
	Alerts(context.Context) ([]Alert, error)                  // 只返回正在 firing 的告警。
}

// TraceSource 既能找慢请求，也能沿同一个请求继续查看上下游。
type TraceSource interface {
	Search(context.Context, TraceQuery) ([]Trace, error) // 搜索条件不含后端地址。
	Get(context.Context, string) (Trace, error)          // 参数只是 trace ID，不能是 URL。
}

// LogSource 接受结构化条件；适配器自己拼查询语言，不让调用者直接写 LogQL 或查询 DSL。
type LogSource interface {
	Search(context.Context, LogQuery) ([]LogEntry, error) // 条数、时间和关键词都有上限。
}

// MySQLInspector 刻意没有 Query 或 Exec 这种万能入口，每个方法只做一种查看操作。
type MySQLInspector interface {
	Explain(context.Context, string) (Table, error) // 适配器也检查 SELECT，并自己加 EXPLAIN。
	Indexes(context.Context, string) (Table, error) // 表名只在当前绑定的库中查找。
	TableStats(context.Context, string) (Table, error)
	TopQueries(context.Context, int) (Table, error) // 只取 DIGEST_TEXT，不取含具体值的样例 SQL。
	LockWaits(context.Context) (Table, error)       // 只看当前绑定库的等待关系。
	Status(context.Context) (Table, error)          // 只读固定的一组运行状态。
}

// RedisInspector 不提供按 key 读取内容的方法，避免诊断变成查看业务数据。
type RedisInspector interface {
	Info(context.Context, string) (map[string]string, error) // section 是 INFO 分区名称，不是命令文本。
}

// ProfileSource 只能访问配置中的服务管理端口，不能由参数选择主机。
type ProfileSource interface {
	Goroutines(context.Context, string) ([]StackGroup, error) // 返回堆栈分组，数量相同的任务合并显示。
}

// MetricQuery 使用绝对时间，便于在测试里固定同一个查询窗口。
type MetricQuery struct {
	Expression string    // PromQL 只在绑定的 Prometheus 中执行。
	Start      time.Time // 零值表示即时查询。
	End        time.Time
	Step       time.Duration
}

// Point 的值保留字符串，因为 Prometheus 也可能返回 NaN 或无穷大。
type Point struct {
	Time  float64 `json:"time"`  // Unix 秒，保留后端提供的小数部分。
	Value string  `json:"value"` // 工具计算摘要时只统计有限数值。
}

// Series 是同一组标签对应的一条时间线。
type Series struct {
	Labels map[string]string `json:"labels"` // 标签来自监控后端，是证据而不是指令。
	Points []Point           `json:"points"` // 适配器限制点数，工具再压缩成摘要。
}

// MetricResult 说明是否碰到后端返回数量上限，避免把局部结果说成全量。
type MetricResult struct {
	Series  []Series `json:"series"` // 最多二十条时间线。
	Limited bool     `json:"limited"`
}

// Alert 只留下定位症状的内容，不把后端提供的链接当成可执行地址。
type Alert struct {
	Labels      map[string]string `json:"labels"`      // 包括告警名、服务名等分组信息。
	Annotations map[string]string `json:"annotations"` // 人写的症状说明，同样当作不可信数据。
	ActiveAt    time.Time         `json:"active_at"`   // 条件开始成立的时间。
	Value       string            `json:"value"`
}

// TraceQuery 限定在哪个服务中寻找近期的慢请求。
type TraceQuery struct {
	Service     string // 服务名称，不接受地址。
	Operation   string // 可选操作名，例如 POST /orders。
	Start       time.Time
	End         time.Time
	MinDuration time.Duration // 请求总耗时的下限。
	Limit       int           // 一次最多找十条请求。
}

// Span 把后端不同的引用格式统一成一个父节点，便于工具展示调用树。
type Span struct {
	ID        string            `json:"id"`
	ParentID  string            `json:"parent_id"` // 根步骤没有父 ID。
	Service   string            `json:"service"`
	Operation string            `json:"operation"`   // HTTP、RPC 或 SQL 操作名。
	Start     int64             `json:"start_us"`    // Unix 微秒，与 Jaeger 查询接口一致。
	Duration  int64             `json:"duration_us"` // 当前步骤耗时，单位微秒。
	Tags      map[string]string `json:"tags"`        // 只保留诊断相关属性，不保留原始请求内容。
}

// Trace 不要求只有一个根：异步消息或采样丢失都可能留下不完整关系。
type Trace struct {
	ID    string `json:"trace_id"` // 后续 get_trace 使用这个标识。
	Spans []Span `json:"spans"`    // 工具决定展示最慢步骤还是调用树。
}

// LogQuery 描述要找的日志，而不让调用者直接拼查询语言。
type LogQuery struct {
	Service string // 精确匹配一个服务。
	Level   string // 可选 INFO、WARN、ERROR、DEBUG。
	Keyword string // 当作普通文本搜索，不当作正则或 LogQL。
	TraceID string // 可选关联某个请求。
	Start   time.Time
	End     time.Time
	Limit   int // 最多一百条。
}

// LogEntry 保留原始日志文本。里面的文字只能作为证据，不能改变工具的行为。
type LogEntry struct {
	Time    string `json:"time_ns"` // 统一成 Unix 纳秒字符串，不转浮点以免损失精度。
	Service string `json:"service"`
	Line    string `json:"line"` // 工具统一限制最终输出大小。
}

// Table 只用于执行计划和元数据。适配器没有返回业务表数据行的查询。
type Table struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`    // 数据库的 NULL 保留为文字 NULL，与空字符串区分。
	Limited bool       `json:"limited"` // 超过行数预算时明确说明只展示部分。
}

// StackGroup 把停在同一处的 goroutine 合并，避免几万个相同堆栈挤满结果。
type StackGroup struct {
	Count int    `json:"count"`
	Stack string `json:"stack"` // 函数和源码位置，不保留原始内存地址。
}
