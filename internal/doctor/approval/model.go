// 本文件：定义一张审批单保存什么，以及执行器需要哪些能力。
//
// 参数和预览一起保存。后续用户批准的是这份已经确定的计划，而不是允许模型以后
// 任意修改同一类资源。重新选参数必须产生另一张审批单。
//
// 主要内容：Plan 保存最终执行参数；Record 保存审批和执行过程；Repository 隔开数据库。
// 阅读建议：执行中的 ConnectionID 是断点线索，不是新的工具参数。

package approval

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// ErrNotFound 只表示没有这张审批；数据库失败不能被误当成可以再创建一张。
var ErrNotFound = errors.New("审批单不存在")

// PoolState 与运维协议对应；医生不导入任何 shop 代码，只通过接口认识这个结构。
type PoolState struct {
	Instance      string `json:"instance"`       // 审批时进程实例。
	Initial       int    `json:"initial"`        // 启动配置，方便人撤销运行时修改。
	Current       int    `json:"current"`        // 审批时真实上限。
	Revision      int    `json:"revision"`       // 审批后变化会让执行失败。
	LastOperation string `json:"last_operation"` // 用于确认回复丢失时是否已执行。
}

// Plan 只由 prepare 生成；HTTP 批准请求不接受这些字段。
type Plan struct {
	Tool     string    `json:"tool"`
	Table    string    `json:"table,omitempty"`
	Columns  []string  `json:"columns,omitempty"`
	Unique   bool      `json:"unique,omitempty"`
	Index    string    `json:"index,omitempty"`
	Service  string    `json:"service,omitempty"`
	Size     int       `json:"size,omitempty"`
	Before   PoolState `json:"before"`
	IDs      []int64   `json:"ids,omitempty"`
	Preview  string    `json:"preview"`
	Rollback string    `json:"rollback"`
}

// Record 的 Args 保留脱敏后的原始申请，Plan 是人审阅与执行的同一份确定内容。
type Record struct {
	ID           string          `json:"id"`
	DiagnosisID  string          `json:"diagnosis_id"`
	Target       string          `json:"target"`
	ParentSeq    int             `json:"parent_seq"`
	CallID       string          `json:"call_id"`
	StepSeq      int             `json:"step_seq"`
	Tool         string          `json:"tool_name"`
	Args         json.RawMessage `json:"args"`
	Plan         Plan            `json:"plan"`
	Reason       string          `json:"reason"`
	Evidence     []int           `json:"evidence_steps"`
	Expected     string          `json:"expected_effect"`
	Risk         string          `json:"risk"`
	Status       string          `json:"status"`
	ExpiresAt    time.Time       `json:"expires_at"`
	DecidedBy    string          `json:"decided_by"`
	DecidedAt    *time.Time      `json:"decided_at"`
	ExecutedAt   *time.Time      `json:"executed_at"`
	ConnectionID int64           `json:"connection_id"`
	Result       tools.Result    `json:"result"`
}

// Repository 的每个写入都检查诊断租约；进程丢失执行权后不能继续操作同一张审批。
type Repository interface {
	FindApproval(context.Context, string, string) (Record, error)
	CreateApproval(context.Context, Record) (Record, error)
	ClaimApproval(context.Context, string, string) (Record, error)
	ApprovalConnection(context.Context, string, string, int64) error
	FinishApproval(context.Context, string, string, tools.Result) error
}

// Metadata 复用原来的 diag_ro 三连接池，只读取列、索引与线程是否存在。
type Metadata interface {
	Columns(context.Context, string) ([]string, error)
	IndexShape(context.Context, string, string) ([]string, bool, bool, error)
	ConnectionAlive(context.Context, int64) (bool, error)
}
