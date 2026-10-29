// 本文件：保存调度状态变化，隔离启动清理、排队和结束操作。
//
// 内存中的取消信号不够。页面重新打开、进程重新启动时，都要能从数据库说明任务
// 为什么没有报告。每次合法转换会一起写 system 步骤，供页面和审计回看。
//
// 主要内容：Recover、Expire 清理旧任务；Pending 选择候选；StartTask、EndTask 保存转换。
// 阅读建议：任何更新都带原状态条件，迟到的完成操作不能把取消结果覆盖成成功。

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
)

// Manual 只负责保存已由接口确认存在的系统，症状先脱敏再落库。
func (store *Store) Manual(ctx context.Context, target, symptoms string) (string, error) {
	id := newID()
	input, _ := json.Marshal(map[string]string{"symptoms": store.config.Redactor.Text(symptoms)})
	_, err := store.database.ExecContext(ctx, "INSERT INTO diagnoses(id,target,`trigger`,status,title,alert_group_key,input) VALUES(?,?,'manual','pending','手动诊断','',?)", id, target, input)
	if err == nil && store.config.Changed != nil {
		store.config.Changed(target, run.Pending, 0)
	}
	return id, err
}

// Pending 先选出每个系统最老的一条，避免某系统大量排队挡住另一系统。
func (store *Store) Pending(ctx context.Context) ([]run.Task, error) {
	rows, err := store.database.QueryContext(ctx, "SELECT id,target,input,created_at FROM (SELECT id,target,input,created_at,ROW_NUMBER() OVER (PARTITION BY target ORDER BY created_at,id) AS position_in_target FROM diagnoses WHERE status='pending') AS candidates WHERE position_in_target=1 ORDER BY created_at,id LIMIT 128")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []run.Task{}
	for rows.Next() {
		var task run.Task
		if err = rows.Scan(&task.ID, &task.Target, &task.Input, &task.CreatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// Recover 不重建对话；阶段 6 的重启必须明确失败，恢复执行留给阶段 7。
func (store *Store) Recover(ctx context.Context) error {
	return store.endSelected(ctx, "SELECT id FROM diagnoses WHERE status='running'", nil, run.Running, run.Failed, "进程重启，恢复功能在阶段 7")
}

// Expire 只清理告警触发的旧 pending，手动任务不会被这条告警时效规则取消。
func (store *Store) Expire(ctx context.Context, cutoff time.Time) error {
	return store.endSelected(ctx, "SELECT id FROM diagnoses WHERE status='pending' AND `trigger`='alert' AND created_at<?", []any{cutoff}, run.Pending, run.Cancelled, "告警已过时，未执行")
}

// endSelected 先关闭结果集再逐条写入，连接池较小时也不占着读连接等待写连接。
func (store *Store) endSelected(ctx context.Context, query string, args []any, from, to, reason string) error {
	rows, err := store.database.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	readErr := rows.Err()
	closeErr := rows.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	// 查询之后任务可能已被另一操作处理，changeState 的条件更新会跳过它。
	for _, id := range ids {
		if _, err = store.changeState(ctx, id, from, to, reason, "finished_at=?,error=?", []any{store.config.Now().UTC(), reason}); err != nil {
			return err
		}
	}
	return nil
}

// StartTask 记录本次选用的模型与提示版本，不能沿用上一个进程的隐含配置。
func (store *Store) StartTask(ctx context.Context, id, model, prompt, provider string) (bool, error) {
	return store.changeState(ctx, id, run.Pending, run.Running, "开始诊断", "started_at=?,model=?,prompt_version=?,provider=?", []any{store.config.Now().UTC(), model, prompt, provider})
}

// EndTask 用累计结果覆盖最终计数，失败时也保存已经发生的用量。
func (store *Store) EndTask(ctx context.Context, id, status string, result agent.Result, reason string) error {
	var report any
	if len(result.Report) > 0 {
		clean, err := store.config.Redactor.JSON(result.Report)
		if err != nil {
			return err
		}
		report = clean
	}
	_, err := store.changeState(ctx, id, run.Running, status, reason, "finished_at=?,report=?,error=?,tokens_in=?,tokens_out=?,cache_hit_tokens=?,cost_micros=?,rounds=?,tool_calls=?,report_mode=?,budget_reason=?", []any{store.config.Now().UTC(), report, store.config.Redactor.Text(reason), result.Usage.Input, result.Usage.Output, result.Usage.CacheHit, result.CostMicros, result.Rounds, result.ToolCalls, result.ReportMode, result.BudgetReason})
	return err
}

// CancelPending 和启动竞争同一行的 pending 状态，只有先拿到行锁的操作成功。
func (store *Store) CancelPending(ctx context.Context, id, reason string) (bool, error) {
	return store.changeState(ctx, id, run.Pending, run.Cancelled, reason, "finished_at=?,error=?", []any{store.config.Now().UTC(), reason})
}

// changeState 仅接受本文件拼出的字段片段，外部内容始终用参数绑定。
func (store *Store) changeState(ctx context.Context, id, from, to, reason, fields string, args []any) (bool, error) {
	if err := run.Transition(from, to); err != nil {
		return false, err
	}
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	values := append([]any{to}, args...)
	values = append(values, id, from)
	result, err := tx.ExecContext(ctx, "UPDATE diagnoses SET status=?,"+fields+" WHERE id=? AND status=?", values...)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if count == 0 {
		return false, nil
	}
	// 状态和 system 步骤一起提交；事件丢了也能从数据库补回。
	payload, _ := json.Marshal(map[string]string{"status": to, "reason": store.config.Redactor.Text(reason)})
	step, err := store.appendStep(ctx, tx, id, agent.Step{Kind: "system", Status: to, Result: string(payload)})
	if err != nil {
		return false, fmt.Errorf("保存状态步骤：%w", err)
	}
	var target string
	var started sql.NullTime
	if err = tx.QueryRowContext(ctx, "SELECT target,started_at FROM diagnoses WHERE id=?", id).Scan(&target, &started); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	store.publish(id, step)
	if store.config.Changed != nil {
		duration := time.Duration(0)
		if started.Valid && to != run.Running {
			duration = max(0, store.config.Now().Sub(started.Time))
		}
		store.config.Changed(target, to, duration)
	}
	return true, nil
}
