// 本文件：读取已经创建的诊断任务，供验收和后续页面使用。
//
// 列表只带摘要，点开详情才加载步骤与报告。任务属于哪个系统，在创建时就确定了，
// 查询接口不提供更换系统的方法。
//
// 主要内容：
//   - Diagnosis、Detail：列表摘要与详情的公开格式。
//   - List：按系统和状态分页，避免一次取回全部历史。
//   - Detail：读取一个任务及其告警快照。
//
// 阅读建议：列表不读取大正文；只有点进详情才加载 input 和告警 JSON。

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
)

// Diagnosis 是列表所需的最小摘要，不包含账号或工具凭据。
type Diagnosis struct {
	ID        string    `json:"id"`         // 稳定任务编号。
	Target    string    `json:"target"`     // 创建时绑定的系统。
	Trigger   string    `json:"trigger"`    // alert 或 manual。
	Status    string    `json:"status"`     // 来自 run 的有限状态表。
	Title     string    `json:"title"`      // 告警分组的展示名。
	CreatedAt time.Time `json:"created_at"` // UTC 创建时间。
	Model     string    `json:"model"`      // 开始时选用的模型；步骤保留实际回复中的模型名。
	Provider  string    `json:"provider"`   // mock 时页面必须醒目标记。
}

// Detail 的嵌入字段使摘要与列表保持一致，避免维护两套字段映射。
type Detail struct {
	LastSeq       int               `json:"last_seq"` // 已提交的最大步骤号，用来拒绝越过当前进度的续传。
	Diagnosis                       // Go 的结构体嵌入：JSON 会把这些公开字段放在同一层。
	Input         json.RawMessage   `json:"input"`            // 初次触发任务的通知。
	Alerts        []json.RawMessage `json:"alerts"`           // 每条告警的最新快照，可能已经 resolved。
	Steps         []agent.Step      `json:"steps"`            // 有序审计步骤，实时新增通过 SSE 接收。
	Report        json.RawMessage   `json:"report"`           // 只有通过校验的报告才保存到这里。
	Error         string            `json:"error"`            // 固定错误或已脱敏的运行错误。
	PromptVersion string            `json:"prompt_version"`   // 本次诊断的提示版本。
	TokensIn      int64             `json:"tokens_in"`        // 累计输入 token。
	TokensOut     int64             `json:"tokens_out"`       // 累计输出 token。
	CacheHit      int64             `json:"cache_hit_tokens"` // 供应商返回的缓存命中 token，保守费用仍按未命中算。
	CostMicros    int64             `json:"cost_micros"`      // 已计入的保守费用。
	Rounds        int               `json:"rounds"`           // 包含最终报告机会的轮数。
	ToolCalls     int               `json:"tool_calls"`       // 处理过的查看工具请求数。
	StartedAt     *time.Time        `json:"started_at"`       // 尚未执行时为 null。
	FinishedAt    *time.Time        `json:"finished_at"`      // 结束前为 null。
}

// List 只把筛选值绑定到参数；排序和 SQL 结构由代码固定。
func (store *Store) List(ctx context.Context, target, status string, limit, offset int) ([]Diagnosis, error) {
	// SQL 片段由程序选择，外部提供的系统名和状态始终通过占位符传入。
	if limit < 1 || limit > 100 || offset < 0 || offset > 100000 {
		return nil, fmt.Errorf("分页范围不合法")
	}
	query := "SELECT id,target,`trigger`,status,title,created_at,COALESCE(model,''),provider FROM diagnoses WHERE 1=1"
	args := []any{}
	if target != "" {
		query += " AND target=?"
		args = append(args, target)
	}
	if status != "" {
		query += " AND status=?"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := store.database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []Diagnosis{}
	for rows.Next() {
		var value Diagnosis
		if err := rows.Scan(&value.ID, &value.Target, &value.Trigger, &value.Status, &value.Title, &value.CreatedAt, &value.Model, &value.Provider); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

// Detail 只按固定 ID 查一次任务，再取相关告警，不允许调用者指定表或字段。
func (store *Store) Detail(ctx context.Context, id string) (Detail, error) {
	// 先查任务再查快照，任务不存在时立刻返回，不能拿空 ID 继续查关联记录。
	value := Detail{Alerts: []json.RawMessage{}}
	var started, finished sql.NullTime
	var rawReport []byte // SQL NULL 要先接到可空字节切片，之后再交给 JSON 字段。
	err := store.database.QueryRowContext(ctx, "SELECT id,target,`trigger`,status,title,created_at,input,COALESCE(model,''),provider,report,COALESCE(error,''),COALESCE(prompt_version,''),tokens_in,tokens_out,cache_hit_tokens,cost_micros,rounds,tool_calls,started_at,finished_at,next_seq FROM diagnoses WHERE id=?", id).Scan(&value.ID, &value.Target, &value.Trigger, &value.Status, &value.Title, &value.CreatedAt, &value.Input, &value.Model, &value.Provider, &rawReport, &value.Error, &value.PromptVersion, &value.TokensIn, &value.TokensOut, &value.CacheHit, &value.CostMicros, &value.Rounds, &value.ToolCalls, &started, &finished, &value.LastSeq)
	if err != nil {
		return value, err
	}
	value.Report = rawReport
	if started.Valid {
		value.StartedAt = &started.Time
	}
	if finished.Valid {
		value.FinishedAt = &finished.Time
	}
	value.Steps, err = store.StepsAfter(ctx, id, 0)
	if err != nil {
		return value, err
	}
	rows, err := store.database.QueryContext(ctx, "SELECT payload FROM diagnosis_alerts WHERE diagnosis_id=? ORDER BY fingerprint", id)
	if err != nil {
		return value, err
	}
	defer rows.Close()
	for rows.Next() {
		var payload json.RawMessage
		if err := rows.Scan(&payload); err != nil {
			return value, err
		}
		value.Alerts = append(value.Alerts, payload)
	}
	return value, rows.Err()
}
