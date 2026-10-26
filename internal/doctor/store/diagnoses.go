// 本文件：读取已经创建的诊断任务，供验收和后续页面使用。
//
// 阶段 5 的任务只会停在 pending。这里能列出任务与告警快照，不启动后台诊断，
// 也不提供更改 target 的方法；任务属于哪个系统，在创建时就确定了。
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
	"encoding/json"
	"fmt"
	"time"
)

// Diagnosis 是列表所需的最小摘要，不包含账号或工具凭据。
type Diagnosis struct {
	ID        string    `json:"id"`         // 稳定任务编号。
	Target    string    `json:"target"`     // 创建时绑定的系统。
	Trigger   string    `json:"trigger"`    // 本阶段固定为 alert。
	Status    string    `json:"status"`     // 本阶段固定为 pending。
	Title     string    `json:"title"`      // 告警分组的展示名。
	CreatedAt time.Time `json:"created_at"` // UTC 创建时间。
}

// Detail 的嵌入字段使摘要与列表保持一致，避免维护两套字段映射。
type Detail struct {
	Diagnosis                   // Go 的结构体嵌入：JSON 会把这些公开字段放在同一层。
	Input     json.RawMessage   `json:"input"`  // 初次触发任务的通知。
	Alerts    []json.RawMessage `json:"alerts"` // 每条告警的最新快照，可能已经 resolved。
}

// List 只把筛选值绑定到参数；排序和 SQL 结构由代码固定。
func (store *Store) List(ctx context.Context, target, status string, limit, offset int) ([]Diagnosis, error) {
	// SQL 片段由程序选择，外部提供的系统名和状态始终通过占位符传入。
	if limit < 1 || limit > 100 || offset < 0 || offset > 100000 {
		return nil, fmt.Errorf("分页范围不合法")
	}
	query := "SELECT id,target,`trigger`,status,title,created_at FROM diagnoses WHERE 1=1"
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
		if err := rows.Scan(&value.ID, &value.Target, &value.Trigger, &value.Status, &value.Title, &value.CreatedAt); err != nil {
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
	err := store.database.QueryRowContext(ctx, "SELECT id,target,`trigger`,status,title,created_at,input FROM diagnoses WHERE id=?", id).Scan(&value.ID, &value.Target, &value.Trigger, &value.Status, &value.Title, &value.CreatedAt, &value.Input)
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
