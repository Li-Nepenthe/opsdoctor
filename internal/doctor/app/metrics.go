// 本文件：记录医生做了多少工作，以及为什么不能继续启动真实诊断。
//
// 指标适合看累计趋势，不能把每个诊断编号都变成标签。系统来自接入配置，工具来自
// 注册表，结果用有限分类。实际模型名可以变化，但最多保留八种，更多归入 other。
//
// 主要内容：newDoctorMetrics 注册仪表；step 记录模型和工具；changed 记录状态；budget 显示额度。
// 阅读建议：请求正文与诊断 ID 在审计里，指标中只有有限维度。

package app

import (
	"sync"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"github.com/prometheus/client_golang/prometheus"
)

// doctorMetrics 的计数器由 Prometheus 库处理并发，mu 只保护有限模型标签集合。
type doctorMetrics struct {
	mu            sync.Mutex               // 保护模型分类，避免供应商任意模型字符串制造无限序列。
	models        map[string]bool          // 最多八个实际模型，其余统一为 other。
	tools         map[string]bool          // 启动时从各系统工具注册表合并，只读。
	diagnoses     *prometheus.CounterVec   // 状态进入次数，按系统与状态区分。
	tokens        *prometheus.CounterVec   // 按模型和输入/输出方向。
	cacheHits     *prometheus.CounterVec   // 供应商声明命中的输入 token。
	costs         *prometheus.CounterVec   // 结算费用及未知预留额，单位人民币元。
	calls         *prometheus.CounterVec   // 查看工具的结果次数。
	toolDuration  *prometheus.HistogramVec // 每次工具耗时。
	duration      *prometheus.HistogramVec // 执行至终态的时间。
	spent         prometheus.Gauge         // 全局账本，包括尚未结算的预留。
	limit         prometheus.Gauge         // 配置的全局上限。
	blocked       prometheus.Gauge         // 0 可启动，1 额度不足。
	blockedReason *prometheus.GaugeVec     // 有限原因分类，便于直接从指标解释为何停止。
}

// newDoctorMetrics 延用平台注册表，诊断耗时另用覆盖五分钟的桶。
func newDoctorMetrics(metrics *telemetry.Metrics, names map[string]bool) (*doctorMetrics, error) {
	// 使用统一注册表添加 service 标签，只有诊断耗时需要单独设置长时间桶。
	value := &doctorMetrics{models: map[string]bool{}, tools: names}
	value.diagnoses = metrics.Counter("doctor_diagnoses_total", "诊断进入各状态的次数。", "target", "status")
	value.tokens = metrics.Counter("doctor_llm_tokens_total", "模型实际 token 用量。", "model", "direction")
	value.cacheHits = metrics.Counter("doctor_llm_cache_hit_tokens_total", "供应商返回的缓存命中输入 token。", "model")
	value.costs = metrics.Counter("doctor_llm_cost_total", "模型结算费用及未知预留额，单位人民币元。", "model")
	value.calls = metrics.Counter("doctor_tool_calls_total", "处理过的查看工具结果。", "tool", "result")
	value.toolDuration = metrics.Histogram("doctor_tool_duration_seconds", "查看工具耗时。", "tool")
	value.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "doctor_diagnosis_duration_seconds", Help: "诊断开始执行至终态的耗时。", Buckets: []float64{1, 5, 15, 30, 60, 120, 180, 240, 300}}, []string{"target", "status"})
	value.spent = prometheus.NewGauge(prometheus.GaugeOpts{Name: "doctor_llm_budget_spent_cny", Help: "全局结算费用及未知预留额与预留额，单位元。"})
	value.limit = prometheus.NewGauge(prometheus.GaugeOpts{Name: "doctor_llm_budget_limit_cny", Help: "全局累计费用上限，单位元。"})
	value.blocked = prometheus.NewGauge(prometheus.GaugeOpts{Name: "doctor_llm_budget_blocked", Help: "真实诊断是否因费用不足停止启动。"})
	value.blockedReason = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "doctor_llm_budget_blocked_reason", Help: "阻止真实诊断的原因，当前原因为1。"}, []string{"reason"})
	for _, collector := range []prometheus.Collector{value.duration, value.spent, value.limit, value.blocked, value.blockedReason} {
		if err := metrics.Register(collector); err != nil {
			return nil, err
		}
	}
	return value, nil
}

// model 将供应商的任意文本限制为八个已见值加 other，实际原文仍完整保存在步骤中。
func (metrics *doctorMetrics) model(name string) string {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if metrics.models[name] {
		return name
	}
	if name != "" && len(name) <= 128 && len(metrics.models) < 8 {
		metrics.models[name] = true
		return name
	}
	return "other"
}

// step 在保存后调用，未知工具和结果都归类，不把模型随手编的名称变成新指标。
func (metrics *doctorMetrics) step(step agent.Step) {
	// 模型与工具各有不同的单位，状态步骤不重复累加用量。
	switch step.Kind {
	case "llm":
		model := metrics.model(step.Model)
		metrics.tokens.WithLabelValues(model, "input").Add(float64(step.Usage.Input))
		metrics.tokens.WithLabelValues(model, "output").Add(float64(step.Usage.Output))
		metrics.cacheHits.WithLabelValues(model).Add(float64(step.Usage.CacheHit))
		metrics.costs.WithLabelValues(model).Add(float64(step.CostMicros) / 1_000_000)
	case "tool":
		name := step.ToolName
		if !metrics.tools[name] && name != "submit_report" {
			name = "unknown"
		}
		status := step.Status
		switch status {
		case "ok", "invalid_argument", "failed", "timeout", "cancelled", "unavailable":
		default:
			status = "other"
		}
		metrics.calls.WithLabelValues(name, status).Inc()
		metrics.toolDuration.WithLabelValues(name).Observe(float64(step.DurationMS) / 1000)
	}
}

// changed 统计状态进入次数，只有开始过执行的终态才计入耗时分布。
func (metrics *doctorMetrics) changed(target, status string, duration time.Duration) {
	metrics.diagnoses.WithLabelValues(target, status).Inc()
	if duration > 0 {
		metrics.duration.WithLabelValues(target, status).Observe(duration.Seconds())
	}
}

// budget 与页面共享调度器读数，额度不足不会只在日志中留下难发现的一句话。
func (metrics *doctorMetrics) budget(value run.BudgetStatus) {
	metrics.spent.Set(float64(value.SpentMicros) / 1_000_000)
	metrics.limit.Set(float64(value.LimitMicros) / 1_000_000)
	blocked := float64(0)
	if value.Blocked {
		blocked = 1
	}
	metrics.blocked.Set(blocked)
	// 每次把旧原因归零；只允许这四种标签，数据库错误文本不会制造新的时间序列。
	code := value.ReasonCode
	switch code {
	case "total_limit", "request_reservation", "accounting_guard":
	default:
		code = "other"
	}
	for _, reason := range []string{"total_limit", "request_reservation", "accounting_guard", "other"} {
		active := float64(0)
		if value.Blocked && code == reason {
			active = 1
		}
		metrics.blockedReason.WithLabelValues(reason).Set(active)
	}
}
