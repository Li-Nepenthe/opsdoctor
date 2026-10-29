// 本文件：定义诊断循环需要的输入、步骤记录和依赖接口。
//
// agent 不知道步骤存在哪张表，只要求 Recorder 能保存步骤并返回编号。这样测试
// 可以用内存记录器，真实运行由 doctor 的仓储保存，再把同一步推送给页面。
//
// 主要内容：Step 是审计单位；Recorder、Toolbox 是使用方接口；Input、Result 描述一次诊断。
// 阅读建议：证据引用的是 Step.Seq，模型工具调用 ID 只负责对话关联。

package agent

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// Step 按诊断内部的 Seq 排序；并行工具按保存完成的次序得到编号。
type Step struct {
	AlreadySaved bool            `json:"-"`                    // 幂等写回命中旧记录时不重复发布事件或累计指标。
	Currency     string          `json:"currency"`             // CNY 或 USD，由仓储账户固定，模型不能指定。
	EventKey     string          `json:"event_key,omitempty"`  // 程序生成的幂等编号，恢复不能把同一结果再记一次。
	ParentSeq    int             `json:"parent_seq,omitempty"` // 工具请求来自哪一步模型回复，避免跨轮重复 ID 混淆。
	CallID       string          `json:"call_id,omitempty"`    // 模型请求的本机编号，或工具对话关联 ID。
	Counted      bool            `json:"counted,omitempty"`    // 是否占普通工具调用额度，恢复后不能重置计数。
	Seq          int             `json:"seq"`                  // 由持久化层分配，不能信任模型传入的数字。
	Kind         string          `json:"kind"`                 // llm、tool、system、approval，分别记录模型、工具、状态和人工审批。
	Model        string          `json:"model,omitempty"`      // 只有 llm 步骤填写实际模型名。
	ToolName     string          `json:"tool_name,omitempty"`  // 工具步骤的注册名。
	Args         json.RawMessage `json:"args"`                 // 实际发出的脱敏请求或工具参数。
	Result       string          `json:"result"`               // 实际回复或有界工具结果，已经脱敏。
	Status       string          `json:"status"`               // ok、invalid_argument、cancelled 等结果。
	DurationMS   int64           `json:"duration_ms"`          // 本步实际耗时。
	Usage        llm.Usage       `json:"usage"`                // llm 的输入输出与缓存命中量。
	CostMicros   int64           `json:"cost_micros"`          // 包含失败尝试保留的预留额。
	CreatedAt    time.Time       `json:"created_at"`           // 数据库保存时间。
}

// Recorder 必须支持并行工具保存，成功返回的步骤才可被报告引用。
type Recorder interface {
	Record(context.Context, Step) (Step, error)
}

// Toolbox 只提供当前系统的查看工具；模型无法改变它绑定的数据源。
type Toolbox interface {
	List() []tools.Definition
	Run(context.Context, string, json.RawMessage) tools.Result
}

// ApprovalFlow 只有申请和读取结果的能力，没有批准按钮。参数里的系统已经由装配层固定。
type ApprovalFlow interface {
	List() []llm.Tool
	Handles(string) bool
	Handle(context.Context, string, int, llm.ToolCall, map[int]bool) (bool, tools.Result, error)
}

// Input 在开始诊断时固定，后续告警仍入库，但不会改变已经发送的固定消息前缀。
type Input struct {
	FaultStartedAt time.Time     // 本次故障的开始时间；例行检查用检查开始时间，不能由模型改写。
	InputOnly      bool          // E0 只准引用最初告警输入，模型回复或工具拒绝消息不能冒充证据。
	Approvals      ApprovalFlow  // 缺省表示只读模式；存在时写操作仍必须由人审批。
	History        []Step        // 已提交的步骤；恢复从这些事实重建，而非重新问一次相同问题。
	ActiveTime     time.Duration // 已经消耗的执行时间，等待审批不计入。
	KnownCost      int64         // 仓储账本中包含未结算预留的费用下界。
	ID             string        // 本次诊断的审计归属。
	Profile        string        // 部署者写的架构说明。
	Unavailable    []string      // 本系统未配置的观测来源。
	Components     []string      // 架构说明列出的组件标识，包含 unknown。
	Symptoms       string        // 最初告警或手动输入；作为不可信数据发送。
	Tools          Toolbox       // 已绑定本系统的注册表。
	Recorder       Recorder      // 持久化与实时推送的出口。
}

// Limits 的默认值来自计划书，测试可调小以快速检查预算边界。
type Limits struct {
	Rounds        int           // 普通模型调用最多 15 轮，触发后另有一次只交报告的机会。
	Tools         int           // 普通阶段最多 40 次工具请求，未知工具也占用额度。
	Tokens        int64         // 累计输入输出含缓存命中，四十万只作为安全兜底。
	ContextTokens int64         // 单次输入加最大输出的估计上限，默认六万四千。
	CostMicros    int64         // 单次诊断三十万最小计账单位，即账户币种的 0.3。
	Duration      time.Duration // 总时限五分钟，包含最后一次报告机会。
	FinalTime     time.Duration // 在总时限中预留一分钟给最后一次模型调用。
}

// DefaultLimits 给正常查证预留四分钟，最后一分钟只整理已有证据，不再查新数据。
func DefaultLimits() Limits {
	return Limits{Rounds: 15, Tools: 40, Tokens: 400000, ContextTokens: 64000, CostMicros: 300000, Duration: 5 * time.Minute, FinalTime: time.Minute}
}

// Result 失败也带用量，费用不能因最终没有报告而消失。
type Result struct {
	Elapsed         time.Duration   // 调度器在同一进程用单调时钟测得，不从两个时间戳相减。
	WaitingApproval bool            // 对话已经保存，等待人决定，不占活动时间。
	Report          json.RawMessage // 校验成功的最终报告，失败时为 nil。
	Rounds          int             // 包含最后一次机会的模型调用数。
	ToolCalls       int             // 实际处理的工具请求数。
	Usage           llm.Usage       // 所有模型回答的累计用量。
	CostMicros      int64           // 累计结算费用。
	BudgetExceeded  bool            // 最后一次机会仍未给出合法报告。
	ReportMode      string          // voluntary 是主动提交，budget 是预算触发后整理。
	BudgetReason    string          // 触发整理的具体上限；主动提交时为空。
}
