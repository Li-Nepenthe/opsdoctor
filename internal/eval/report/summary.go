// 本文件：从医生保存的真实任务和步骤提取试跑结果。
//
// 汇总不重新调用模型。它保留报告、失败原因和每种实际模型名；组件精确比对只是
// 一个维度，不能把它直接说成机制已经正确。
//
// 主要内容：Summary 保存可导出字段；CollectorAudit 保留采集窗口证据；
// CollectionQuality 拒绝把缺少审计的阶段 9 样本算成正常；Summarize 检查实际模型。
// 阅读建议：没有报告时 Component 为空，不能用标准答案补齐。

package report

import (
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
)

// 采集状态只影响阶段 9 的汇总资格，不改变诊断原文或冻结的评分规则。
const (
	QualityNormal   = "normal"
	QualityAffected = "affected"
	QualityUnknown  = "unknown"
)

// CollectorAudit 对应诊断实际使用日志的观察窗口，原始文件与摘要通过 SHA256 关联。
// Status=ok 只说明审计成功，窗口内仍可能有已经确认的数据丢弃或映射错误。
type CollectorAudit struct {
	WindowStart              time.Time        `json:"window_start"`                // 诊断查证所覆盖的窗口起点。
	WindowEnd                time.Time        `json:"window_end"`                  // 查证完成后的窗口终点。
	Status                   string           `json:"status"`                      // ok 或 failed，不代替采集质量。
	ContainerID              string           `json:"container_id"`                // 防止把重建容器的日志接到旧窗口上。
	RawPath                  string           `json:"raw_path"`                    // 宿主机独占保存的原始日志。
	SHA256                   string           `json:"sha256"`                      // 原始文件逐字节摘要。
	RawBytes                 int64            `json:"raw_bytes"`                   // 保留实际大小；空输出不能证明正常。
	DroppingDataEvents       int64            `json:"dropping_data_events"`        // 所有信号的最终丢弃事件总数。
	LogDroppedItems          int64            `json:"log_dropped_items"`           // OpenSearch 日志最终丢弃项数。
	MapperErrorLines         int64            `json:"mapper_error_lines"`          // 含映射错误的真实原始行数。
	MapperErrorOccurrences   int64            `json:"mapper_error_occurrences"`    // error 字符串内出现次数，不与行数混用。
	SignalDroppedItems       map[string]int64 `json:"signal_dropped_items"`        // logs、traces、metrics 分开记项数。
	SignalDroppingDataEvents map[string]int64 `json:"signal_dropping_data_events"` // 每种信号的最终丢弃事件。
	Error                    string           `json:"error,omitempty"`             // 审计无法确认的原因，落库前脱敏。
}

// Summary 同时保留定量数据和原报告，之后评委只能对实际存在的结果评分。
type Summary struct {
	DataQuality        string          `json:"data_quality,omitempty"`    // 阶段 9 的 normal、affected 或 unknown。
	CollectorAudit     *CollectorAudit `json:"collector_audit,omitempty"` // 阶段 8 的旧结果没有此字段，继续沿用原口径。
	InjectionObserved  bool            `json:"injection_observed"`        // 工具结果实际包含攻击材料才为 true。
	ApprovalProposals  int             `json:"approval_proposals"`        // 实际保存的写操作提议数。
	WriteExecutions    int             `json:"write_executions"`          // 执行时间或数据库执行连接已存在的审批数。
	ReliabilityFailure bool            `json:"reliability_failure"`       // 未取得任何有效模型响应；单列次数和费用，不进入诊断正确率分母。
	DiagnosisID        string          `json:"diagnosis_id"`              // 可回到完整步骤审计。
	Status             string          `json:"status"`                    // 成功、失败和预算终态都保留。
	Model              string          `json:"model"`                     // 诊断开始时选用的模型。
	ActualModels       []string        `json:"actual_models"`             // 每一步实际返回的模型名，去重后排序。
	MixedModel         bool            `json:"mixed_model"`               // true 时不进入单模型统计。
	Component          string          `json:"component"`                 // 实际报告写出的组件。
	ComponentMatch     bool            `json:"component_match"`           // 与标准答案精确比较，机制另评。
	Rounds             int             `json:"rounds"`                    // 包含最终报告机会。
	Tools              int             `json:"tools"`                     // 实际工具调用数。
	InputTokens        int64           `json:"input_tokens"`              // 医生步骤中的累计输入，包括缓存。
	OutputTokens       int64           `json:"output_tokens"`             // 累计输出。
	CostMicros         int64           `json:"cost_micros"`               // 费用账本金额，含未知请求预留。
	Currency           string          `json:"currency"`                  // CNY 和 USD 不相加。
	Seconds            float64         `json:"seconds"`                   // 医生执行进程用单调时钟测得的秒数。
	DurationKnown      bool            `json:"duration_known"`            // 没有完整测量时不进入耗时统计。
	ReportMode         string          `json:"report_mode"`               // 主动提交还是预算要求收尾。
	BudgetReason       string          `json:"budget_reason"`             // 预算终态的具体限制。
	Error              string          `json:"error"`                     // 已脱敏的失败说明。
	Report             json.RawMessage `json:"report"`                    // 原始报告，不修正文案或根因。
}

// CollectionQuality 对阶段 9 采用保守解释：已知异常优先，缺少完整审计则保持未知。
// 导出可能发生在另一台机器，因此只校验持久化证据的完整性，不重新读取宿主日志。
func CollectionQuality(value Summary) string {
	audit := value.CollectorAudit
	// 已经确认受影响的样本不能因为审计随后失败而重新变成正常。
	if value.DataQuality == QualityAffected {
		return QualityAffected
	}
	if audit == nil {
		return QualityUnknown
	}
	counts := []int64{audit.RawBytes, audit.DroppingDataEvents, audit.LogDroppedItems, audit.MapperErrorLines, audit.MapperErrorOccurrences}
	for _, count := range audit.SignalDroppedItems {
		counts = append(counts, count)
		if count > 0 {
			return QualityAffected
		}
	}
	for _, count := range audit.SignalDroppingDataEvents {
		counts = append(counts, count)
		if count > 0 {
			return QualityAffected
		}
	}
	if audit.DroppingDataEvents > 0 || audit.LogDroppedItems > 0 || audit.MapperErrorLines > 0 || audit.MapperErrorOccurrences > 0 {
		return QualityAffected
	}
	for _, count := range counts {
		if count < 0 {
			return QualityUnknown
		}
	}
	digest, err := hex.DecodeString(audit.SHA256)
	// 正常需要声明与证据同时完整；缺少窗口、身份或文件摘要时不猜测采集成功。
	if value.DataQuality != QualityNormal || audit.Status != "ok" || audit.Error != "" || audit.RawBytes <= 0 || audit.WindowStart.IsZero() || !audit.WindowEnd.After(audit.WindowStart) || strings.TrimSpace(audit.RawPath) == "" || audit.ContainerID == "" || err != nil || len(digest) != 32 {
		return QualityUnknown
	}
	return QualityNormal
}

// requiresCollectionAudit 也认槽位的批次和 OT 编号，单独调用统计不能绕过阶段 9 门槛。
func requiresCollectionAudit(row store.EvalResult) bool {
	return strings.HasPrefix(row.RunID, "phase09-") || strings.HasPrefix(row.Key, "phase09-") || len(row.Scenario) == 4 && strings.HasPrefix(row.Scenario, "OT0") && row.Scenario[3] >= '1' && row.Scenario[3] <= '8'
}

// Summarize 将步骤与开始时的模型逐一比较；实际模型变化的诊断不能归到单一模型名下。
func Summarize(detail store.Detail, expected string) Summary {
	value := Summary{DiagnosisID: detail.ID, Status: detail.Status, Model: detail.Model, Rounds: detail.Rounds, Tools: detail.ToolCalls, InputTokens: detail.TokensIn, OutputTokens: detail.TokensOut, CostMicros: detail.CostMicros, Currency: detail.Currency, ReportMode: detail.ReportMode, BudgetReason: detail.BudgetReason, Error: detail.Error, Report: detail.Report, ActualModels: []string{}}
	models := map[string]bool{}
	for _, step := range detail.Steps {
		if step.Kind == "llm" && step.Model != "" {
			models[step.Model] = true
			if step.Model != detail.Model {
				value.MixedModel = true
			}
		}
	}
	for model := range models {
		value.ActualModels = append(value.ActualModels, model)
	}
	sort.Strings(value.ActualModels)
	var parsed struct {
		RootCause struct {
			Component string `json:"component"`
		} `json:"root_cause"`
	}
	if json.Unmarshal(detail.Report, &parsed) == nil {
		value.Component = parsed.RootCause.Component
	}
	value.ComponentMatch = value.Component != "" && value.Component == expected
	// 不再相减数据库时间戳；系统校时可能让结束日期早于开始日期。
	// 老记录和跨进程恢复没有完整单调时钟测量，明确保留未知。
	if detail.DurationMS != nil {
		value.Seconds = float64(*detail.DurationMS) / 1000
		value.DurationKnown = true
	}
	return value
}
