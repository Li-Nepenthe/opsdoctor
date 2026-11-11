// 本文件：读取候选任务并保存调度状态。状态更新核对旧状态，同时记下原因和步骤，防止迟到的完成操作覆盖取消结果。

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
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
	rows, err := store.database.QueryContext(ctx, "SELECT id,target,input,created_at,COALESCE(fault_started_at,created_at),eval_group,eval_expected_model,eval_profile FROM (SELECT id,target,input,created_at,fault_started_at,eval_group,eval_expected_model,eval_profile,ROW_NUMBER() OVER (PARTITION BY target ORDER BY CASE status WHEN 'running' THEN 0 WHEN 'waiting_approval' THEN 1 ELSE 2 END,created_at,id) AS position_in_target FROM diagnoses WHERE `trigger`<>'judge' AND (status='pending' OR (status='running' AND lease_until<=UTC_TIMESTAMP(6)) OR (status='waiting_approval' AND NOT EXISTS(SELECT 1 FROM approvals WHERE diagnosis_id=diagnoses.id AND status='pending'))) AND (`trigger`='eval' OR NOT EXISTS(SELECT 1 FROM evaluation_targets WHERE target=diagnoses.target AND enabled=TRUE))) AS candidates WHERE position_in_target=1 ORDER BY created_at,id LIMIT 128")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []run.Task{}
	for rows.Next() {
		var task run.Task
		if err = rows.Scan(
			&task.ID, &task.Target, &task.Input, &task.CreatedAt,
			&task.FaultStartedAt, &task.EvaluationGroup, &task.ExpectedModel, &task.ExecutionProfile,
		); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// Recover 只结束中断太久的任务；有效的过期租约由调度器接手并重建对话。
func (store *Store) Recover(ctx context.Context) error {
	if err := store.ExpireApprovals(ctx); err != nil {
		return err
	}
	return store.staleLeases(ctx)
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
		if _, err = store.changeState(ctx, id, stateChange{
			from: from, to: to, reason: reason,
			fields: "finished_at=UTC_TIMESTAMP(6),error=?",
			args:   []any{reason},
		}); err != nil {
			return err
		}
	}
	return nil
}

// StartTask 记录本次选用的模型与提示版本，不能沿用上一个进程的隐含配置。
func (store *Store) StartTask(ctx context.Context, id, model, prompt, provider string) (bool, error) {
	return store.claimLease(ctx, id, model, prompt, provider)
}

// EndTask 用累计结果覆盖最终计数，失败时也保存已经发生的用量。
func (store *Store) EndTask(ctx context.Context, id, status string, result agent.Result, reason string) error {
	if status == run.WaitingApproval {
		_, err := store.changeState(ctx, id, stateChange{
			from: run.Running, to: status, reason: reason,
			fields: "active_ms=active_ms+GREATEST(0,TIMESTAMPDIFF(MICROSECOND,lease_updated_at,UTC_TIMESTAMP(6)) DIV 1000),rounds=?,tool_calls=?",
			args:   []any{result.Rounds, result.ToolCalls},
		})
		return err
	}
	var report any
	if len(result.Report) > 0 {
		clean, err := store.config.Redactor.JSON(result.Report)
		if err != nil {
			return err
		}
		report = clean
	}
	// 崩溃前的单调时钟随进程消失。多段执行没有完整测量，就保留 NULL，
	// 不能拿最后一段冒充整个诊断。评测仍保留报告，但不把未知时长算进平均值。
	_, err := store.changeState(ctx, id, stateChange{
		from: run.Running, to: status, reason: reason,
		fields: "finished_at=UTC_TIMESTAMP(6),duration_ms=IF(execution_segments=1,?,NULL),report=?,error=?,tokens_in=?,tokens_out=?,cache_hit_tokens=?,cost_micros=?,rounds=?,tool_calls=?,report_mode=?,budget_reason=?",
		args:   []any{max(0, result.Elapsed.Milliseconds()), report, store.config.Redactor.Text(reason), result.Usage.Input, result.Usage.Output, result.Usage.CacheHit, result.CostMicros, result.Rounds, result.ToolCalls, result.ReportMode, result.BudgetReason},
	})
	return err
}

// CancelPending 和启动竞争同一行的 pending 状态，只有先拿到行锁的操作成功。
func (store *Store) CancelPending(ctx context.Context, id, reason string) (bool, error) {
	changed, err := store.changeState(ctx, id, stateChange{
		from: run.Pending, to: run.Cancelled, reason: reason,
		fields: "finished_at=UTC_TIMESTAMP(6),error=?",
		args:   []any{reason},
	})
	if err != nil || changed {
		return changed, err
	}
	changed, err = store.changeState(ctx, id, stateChange{
		from: run.WaitingApproval, to: run.Cancelled, reason: reason,
		fields: "finished_at=UTC_TIMESTAMP(6),error=?,cancel_requested=TRUE",
		args:   []any{reason},
	})
	if err != nil || changed {
		return changed, err
	}
	result, err := store.database.ExecContext(ctx, "UPDATE diagnoses SET cancel_requested=TRUE WHERE id=? AND status IN ('running','waiting_approval') AND cancel_requested=FALSE", id)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

// stateChange 只接收代码内固定的更新字段，字段对应的业务值必须按顺序单独绑定。
type stateChange struct {
	from   string
	to     string
	reason string
	fields string
	args   []any
}

// changeState 把状态和系统步骤放在同一事务内，只在提交成功后向订阅者发布。
func (store *Store) changeState(ctx context.Context, id string, change stateChange) (bool, error) {
	if err := run.Transition(change.from, change.to); err != nil {
		return false, err
	}
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if owner := llm.Lease(ctx); owner != "" {
		if err = requireLease(ctx, tx, id, owner); err != nil {
			return false, err
		}
	}
	values := append([]any{change.to}, change.args...)
	values = append(values, id, change.from)
	result, err := tx.ExecContext(ctx, "UPDATE diagnoses SET status=?,"+change.fields+" WHERE id=? AND status=?", values...)
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
	payload, _ := json.Marshal(map[string]string{"status": change.to, "reason": store.config.Redactor.Text(change.reason)})
	step, err := store.appendStep(ctx, tx, id, agent.Step{Kind: "system", Status: change.to, Result: string(payload)})
	if err != nil {
		return false, fmt.Errorf("保存状态步骤：%w", err)
	}
	// 先保存状态步骤，再释放租约；反过来会让本事务自己的步骤被租约校验拒绝。
	if change.from == run.Running && change.to != run.Running {
		if _, err = tx.ExecContext(ctx, "UPDATE diagnoses SET lease_owner=NULL,lease_until=NULL WHERE id=?", id); err != nil {
			return false, err
		}
	}
	var target string
	var elapsed sql.NullInt64
	if err = tx.QueryRowContext(ctx, "SELECT target,duration_ms FROM diagnoses WHERE id=?", id).Scan(&target, &elapsed); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	store.publish(id, step)
	if store.config.Changed != nil {
		duration := time.Duration(0)
		if elapsed.Valid && change.to != run.Running {
			duration = time.Duration(elapsed.Int64) * time.Millisecond
		}
		store.config.Changed(target, change.to, duration)
	}
	return true, nil
}
