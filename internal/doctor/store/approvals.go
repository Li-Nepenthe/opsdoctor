// 本文件：保存人工审批，并用条件更新保证同一张单只能批准或拒绝一次。
//
// 审批接口只能给出“同意还是拒绝”。SQL 更新不接受新参数，所以审批之后也不能
// 换掉原来的索引、连接池值或商品编号。执行状态另行保存，重启后先看真实结果。
//
// 主要内容：CreateApproval 保存计划；DecideApproval 抢占决定权；ClaimApproval 领取执行；
// FinishApproval 保存结果；ExpireApprovals 让过期申请也能交回模型。
// 阅读建议：写入先锁诊断行，再锁审批行，与执行租约遵循同样顺序。

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/approval"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// FindApproval 总是同时限定诊断编号，不能拿另一个诊断的审批结果接续当前任务。
func (store *Store) FindApproval(ctx context.Context, diagnosis, id string) (approval.Record, error) {
	row := store.database.QueryRowContext(ctx, approvalSelect+" WHERE a.diagnosis_id=? AND a.id=?", diagnosis, id)
	return scanApproval(row)
}

// approvalSelect 所有查询共用固定列序，JSON 计划只读，不允许调用方拼字段。
const approvalSelect = `SELECT a.id,a.diagnosis_id,d.target,a.parent_seq,a.call_id,a.step_seq,a.tool_name,a.args,a.plan,a.reason,a.evidence_steps,a.expected_effect,a.risk,a.status,a.expires_at,a.decided_by,a.decided_at,a.executed_at,a.connection_id,a.result FROM approvals a JOIN diagnoses d ON d.id=a.diagnosis_id`

// scanner 让 QueryRow 和 Rows 使用同一套解码，避免列表与执行读出不同参数。
type scanner interface{ Scan(...any) error }

// scanApproval 不给损坏的 JSON 补默认值，无法准确恢复参数时必须停下。
func scanApproval(row scanner) (approval.Record, error) {
	var value approval.Record
	var plan, evidence, result []byte
	var decided, executed sql.NullTime
	err := row.Scan(&value.ID, &value.DiagnosisID, &value.Target, &value.ParentSeq, &value.CallID, &value.StepSeq, &value.Tool, &value.Args, &plan, &value.Reason, &evidence, &value.Expected, &value.Risk, &value.Status, &value.ExpiresAt, &value.DecidedBy, &decided, &executed, &value.ConnectionID, &result)
	if errors.Is(err, sql.ErrNoRows) {
		return value, approval.ErrNotFound
	}
	if err != nil {
		return value, err
	}
	if json.Unmarshal(plan, &value.Plan) != nil || json.Unmarshal(evidence, &value.Evidence) != nil {
		return value, fmt.Errorf("审批参数损坏，不能执行")
	}
	if len(result) > 0 && json.Unmarshal(result, &value.Result) != nil {
		return value, fmt.Errorf("审批结果损坏")
	}
	// NULL 表示还没有人决定或执行，页面不能把零时间显示成真实操作时间。
	if decided.Valid {
		value.DecidedAt = &decided.Time
	}
	if executed.Valid {
		value.ExecutedAt = &executed.Time
	}
	return value, nil
}

// Approvals 返回本次诊断的审批单，页面只能通过它展示已经保存的内容。
func (store *Store) Approvals(ctx context.Context, diagnosis string) ([]approval.Record, error) {
	rows, err := store.database.QueryContext(ctx, approvalSelect+" WHERE a.diagnosis_id=? ORDER BY a.step_seq LIMIT 40", diagnosis)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []approval.Record{}
	for rows.Next() {
		value, err := scanApproval(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

// CreateApproval 在同一事务中保存计划和页面步骤；崩溃时不会只保存其中一半。
func (store *Store) CreateApproval(ctx context.Context, value approval.Record) (approval.Record, error) {
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return value, err
	}
	defer tx.Rollback()
	if err = requireLease(ctx, tx, value.DiagnosisID, llm.Lease(ctx)); err != nil {
		return value, err
	}
	var target string
	if err = tx.QueryRowContext(ctx, "SELECT target FROM diagnoses WHERE id=?", value.DiagnosisID).Scan(&target); err != nil {
		return value, err
	}
	if target != value.Target {
		return value, fmt.Errorf("审批系统与诊断不符")
	}
	// 计划和可回放步骤一起提交，避免崩溃后页面有申请、数据库却没有执行依据。
	plan, _ := json.Marshal(value.Plan)
	evidence, _ := json.Marshal(value.Evidence)
	_, err = tx.ExecContext(ctx, `INSERT INTO approvals(id,diagnosis_id,parent_seq,call_id,tool_name,args,plan,reason,evidence_steps,expected_effect,risk,expires_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,UTC_TIMESTAMP(6)+INTERVAL 30 MINUTE) ON DUPLICATE KEY UPDATE id=id`, value.ID, value.DiagnosisID, value.ParentSeq, value.CallID, value.Tool, value.Args, plan, value.Reason, evidence, value.Expected, value.Risk)
	if err != nil {
		return value, err
	}
	payload, _ := json.Marshal(map[string]string{"approval_id": value.ID, "status": "pending", "message": "等待人工审批，尚未执行"})
	step, err := store.appendStep(ctx, tx, value.DiagnosisID, agent.Step{EventKey: "approval:" + value.ID, ParentSeq: value.ParentSeq, CallID: value.CallID, Kind: "approval", ToolName: value.Tool, Args: value.Args, Result: string(payload), Status: "pending"})
	if err != nil {
		return value, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE approvals SET step_seq=? WHERE id=?", step.Seq, value.ID); err != nil {
		return value, err
	}
	if err = tx.Commit(); err != nil {
		return value, err
	}
	store.publish(value.DiagnosisID, step)
	return store.FindApproval(ctx, value.DiagnosisID, value.ID)
}

// DecideApproval 只有一个请求能把仍有效的 pending 单改掉，HTTP 重试不会二次决定。
func (store *Store) DecideApproval(ctx context.Context, id, user string, approved bool) (bool, error) {
	if user == "" || len(user) > 128 {
		return false, fmt.Errorf("审批缺少登录身份")
	}
	var diagnosis string
	if err := store.database.QueryRowContext(ctx, "SELECT diagnosis_id FROM approvals WHERE id=?", id).Scan(&diagnosis); err != nil {
		return false, err
	}
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	// 先锁诊断，防止用户取消与批准并发时还放行写操作。
	var active bool
	if err = tx.QueryRowContext(ctx, "SELECT status IN ('running','waiting_approval') AND cancel_requested=FALSE FROM diagnoses WHERE id=? FOR UPDATE", diagnosis).Scan(&active); err != nil {
		return false, err
	}
	if !active {
		return false, nil
	}
	state := "rejected"
	if approved {
		state = "approved"
	}
	changed, err := tx.ExecContext(ctx, "UPDATE approvals SET status=?,decided_by=?,decided_at=UTC_TIMESTAMP(6) WHERE id=? AND status='pending' AND expires_at>UTC_TIMESTAMP(6)", state, user, id)
	if err != nil {
		return false, err
	}
	count, err := changed.RowsAffected()
	if err != nil || count != 1 {
		return false, err
	}
	payload, _ := json.Marshal(map[string]string{"approval_id": id, "status": state, "decided_by": user})
	step, err := store.appendStep(ctx, tx, diagnosis, agent.Step{Kind: "approval", Status: state, Result: string(payload)})
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	store.publish(diagnosis, step)
	return true, nil
}

// ClaimApproval 把批准与执行分开记录，已在执行的单子只能由当前诊断租约的持有者恢复。
func (store *Store) ClaimApproval(ctx context.Context, diagnosis, id string) (approval.Record, error) {
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return approval.Record{}, err
	}
	defer tx.Rollback()
	if err = requireLease(ctx, tx, diagnosis, llm.Lease(ctx)); err != nil {
		return approval.Record{}, err
	}
	var state, owner string
	var cancelled bool
	if err = tx.QueryRowContext(ctx, "SELECT cancel_requested FROM diagnoses WHERE id=?", diagnosis).Scan(&cancelled); err != nil {
		return approval.Record{}, err
	}
	if cancelled {
		return approval.Record{}, fmt.Errorf("诊断已取消，不再执行审批")
	}
	if err = tx.QueryRowContext(ctx, "SELECT status,execution_owner FROM approvals WHERE id=? AND diagnosis_id=? FOR UPDATE", id, diagnosis).Scan(&state, &owner); err != nil {
		return approval.Record{}, err
	}
	// 相同租约不能同时启动两次执行；新租约才允许接手并核对远端状态。
	if state == "executing" && owner == llm.Lease(ctx) {
		return approval.Record{}, fmt.Errorf("同一执行者已领取审批，不能并发重做")
	}
	if state != "approved" && state != "executing" {
		return approval.Record{}, fmt.Errorf("审批未经批准或已经执行")
	}
	if _, err = tx.ExecContext(ctx, "UPDATE approvals SET status='executing',execution_owner=? WHERE id=?", llm.Lease(ctx), id); err != nil {
		return approval.Record{}, err
	}
	if err = tx.Commit(); err != nil {
		return approval.Record{}, err
	}
	return store.FindApproval(ctx, diagnosis, id)
}

// ApprovalConnection 在发送 DDL 前保存连接编号，恢复者可以先检查旧操作是否还在运行。
func (store *Store) ApprovalConnection(ctx context.Context, diagnosis, id string, connection int64) error {
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = requireLease(ctx, tx, diagnosis, llm.Lease(ctx)); err != nil {
		return err
	}
	// 先保存连接编号，才允许发出 DDL；恢复进程可据此辨认未结束的旧操作。
	res, err := tx.ExecContext(ctx, "UPDATE approvals SET connection_id=? WHERE id=? AND diagnosis_id=? AND status='executing'", connection, id, diagnosis)
	if err != nil {
		return err
	}
	if count, _ := res.RowsAffected(); count != 1 {
		return fmt.Errorf("审批执行状态已变化")
	}
	return tx.Commit()
}

// FinishApproval 只有当前租约能保存结果；旧执行者的迟到回复不会盖掉恢复者的核对。
func (store *Store) FinishApproval(ctx context.Context, diagnosis, id string, result tools.Result) error {
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = requireLease(ctx, tx, diagnosis, llm.Lease(ctx)); err != nil {
		return err
	}
	// 只有确定成功才标 executed；结果不明要保留在单上供人工核对。
	state := "failed"
	if result.Status == "ok" {
		state = "executed"
	}
	raw, _ := json.Marshal(result)
	res, err := tx.ExecContext(ctx, "UPDATE approvals SET status=?,executed_at=UTC_TIMESTAMP(6),result=? WHERE id=? AND diagnosis_id=? AND status='executing'", state, raw, id, diagnosis)
	if err != nil {
		return err
	}
	if count, _ := res.RowsAffected(); count != 1 {
		return fmt.Errorf("审批已经结束或执行权变化")
	}
	payload, _ := json.Marshal(map[string]string{"approval_id": id, "status": state})
	step, err := store.appendStep(ctx, tx, diagnosis, agent.Step{Kind: "approval", Status: state, Result: string(payload)})
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	store.publish(diagnosis, step)
	return nil
}

// ExpireApprovals 不需要人在页面点拒绝；到期后任务会被调度器继续，让模型得到明确的未执行结果。
func (store *Store) ExpireApprovals(ctx context.Context) error {
	rows, err := store.database.QueryContext(ctx, "SELECT id,diagnosis_id FROM approvals WHERE status='pending' AND expires_at<=UTC_TIMESTAMP(6) LIMIT 128")
	if err != nil {
		return err
	}
	type item struct{ id, diagnosis string }
	var expired []item
	for rows.Next() {
		var value item
		if err = rows.Scan(&value.id, &value.diagnosis); err != nil {
			_ = rows.Close()
			return err
		}
		expired = append(expired, value)
	}
	readErr := rows.Err()
	_ = rows.Close()
	if readErr != nil {
		return readErr
	}
	// 先关结果集，再逐张事务处理。与人工批准争同一个 pending 条件，只有一方能成功。
	for _, value := range expired {
		if err = store.expireApproval(ctx, value.id, value.diagnosis); err != nil {
			return err
		}
	}
	return nil
}

// expireApproval 与人工决定使用相同的锁顺序，过期状态和页面事件一起落库。
func (store *Store) expireApproval(ctx context.Context, id, diagnosis string) error {
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var locked string
	if err = tx.QueryRowContext(ctx, "SELECT id FROM diagnoses WHERE id=? FOR UPDATE", diagnosis).Scan(&locked); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, "UPDATE approvals SET status='expired',decided_at=UTC_TIMESTAMP(6) WHERE id=? AND status='pending' AND expires_at<=UTC_TIMESTAMP(6)", id)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil || count != 1 {
		return err
	}
	// 不再要求用户刷新后自行猜测，SSE 和历史都会明确记录“过期，未执行”。
	payload, _ := json.Marshal(map[string]string{"approval_id": id, "status": "expired", "message": "审批已过期，未执行"})
	step, err := store.appendStep(ctx, tx, diagnosis, agent.Step{EventKey: "approval-expired:" + id, Kind: "approval", Status: "expired", Result: string(payload)})
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	store.publish(diagnosis, step)
	return nil
}
