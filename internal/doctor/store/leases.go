// 本文件：在数据库内分配诊断名额，续租或接手已有的诊断。
// claimLease 锁住协调行，用数据库时钟检查租约。接手者换用新的编号，requireLease 据此阻止旧进程写入。
// Renew 累计执行时间，staleLeases 清理证据过时的任务，Resume 读回全部步骤和已占用的费用。

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// requireLease 必须在持有诊断行锁的事务中调用，检查和写入之间不能让别人接手。
func requireLease(ctx context.Context, tx *sql.Tx, id, owner string) error {
	var valid bool
	err := tx.QueryRowContext(ctx, "SELECT COALESCE(lease_owner=? AND lease_until>UTC_TIMESTAMP(6),FALSE) FROM diagnoses WHERE id=? FOR UPDATE", owner, id).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("诊断租约已丢失，停止旧执行者")
	}
	return nil
}

type claimRequest struct {
	id, model, prompt, provider, owner string
}

type claimRow struct {
	target, status, currency, trigger                 string
	expectedModel, profile, expectedProvider, account string
	available, cancelled, recent, evaluating          bool
	undecided, sameTarget, active                     int
}

// claimLease 用协调行把全局两个名额和同一系统的一个名额一起锁住，提交后才通知调度器。
func (store *Store) claimLease(ctx context.Context, id, model, prompt, provider string) (bool, error) {
	request := claimRequest{id: id, model: model, prompt: prompt, provider: provider, owner: llm.Lease(ctx)}
	if request.owner == "" {
		return false, fmt.Errorf("开始诊断缺少租约编号")
	}
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	row, eligible, err := store.readClaim(ctx, tx, request)
	if err != nil || !eligible {
		return false, err
	}
	if err := row.readSlots(ctx, tx, request.id); err != nil {
		return false, err
	}
	if !claimSlotsAvailable(row) {
		return false, nil
	}
	return store.commitClaim(ctx, tx, request, row)
}

func (store *Store) readClaim(ctx context.Context, tx *sql.Tx, request claimRequest) (claimRow, bool, error) {
	var row claimRow
	var guard int
	if err := tx.QueryRowContext(ctx, "SELECT id FROM scheduler_guard WHERE id=1 FOR UPDATE").Scan(&guard); err != nil {
		return row, false, err
	}
	err := tx.QueryRowContext(ctx, lockLeaseDiagnosisSQL,
		-store.config.ResumeMaxAge.Microseconds(), request.id,
	).Scan(
		&row.target, &row.status, &row.available, &row.cancelled, &row.currency, &row.recent,
		&row.trigger, &row.expectedModel, &row.profile, &row.expectedProvider, &row.account,
	)
	if err != nil {
		return row, false, err
	}
	row.evaluating, err = evaluationEnabled(ctx, tx, row.target)
	if err != nil {
		return row, false, err
	}
	eligible, err := claimRowEligible(row, store.config, request)
	return row, eligible, err
}

func (row *claimRow) readSlots(ctx context.Context, tx *sql.Tx, id string) error {
	if row.status == "waiting_approval" {
		err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM approvals WHERE diagnosis_id=? AND status='pending'", id).Scan(&row.undecided)
		if err != nil || row.undecided > 0 {
			return err
		}
	}
	// 等待审批也占用该系统名额；全局执行名额只数仍持有有效租约的 running。
	var occupied int
	err := tx.QueryRowContext(ctx, occupiedTargetSlotsSQL, row.target, id).Scan(&occupied, &row.sameTarget)
	if err != nil || row.sameTarget > 0 {
		return err
	}
	return tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM diagnoses WHERE id<>? AND status='running' AND lease_until>UTC_TIMESTAMP(6)", id).Scan(&row.active)
}

func claimRowEligible(row claimRow, config Config, request claimRequest) (bool, error) {
	if ok, err := claimEvaluationAllowed(row, config, request); !ok {
		return false, err
	}
	if row.status != "pending" && row.currency != config.Currency {
		return false, fmt.Errorf("不能跨币种恢复诊断，请保持原账户或取消后重新发起")
	}
	if row.cancelled {
		return false, nil
	}
	switch row.status {
	case "pending", "waiting_approval":
		return true, nil
	case "running":
		// 即使调度器尚未清理，过时证据也不能用于接手。
		return row.available && row.recent, nil
	default:
		return false, nil
	}
}

func claimSlotsAvailable(row claimRow) bool {
	return row.undecided <= 0 && row.sameTarget <= 0 && row.active < 2
}

func (store *Store) commitClaim(ctx context.Context, tx *sql.Tx, request claimRequest, row claimRow) (bool, error) {
	// 中断到租约过期的时间也保守计入执行预算；接手后的等待不能重新拿五分钟。
	// 运行过但段数为零的任务已有两段执行，接手时补记为两段，避免被当作单段计算耗时。
	_, err := tx.ExecContext(ctx, claimLeaseSQL, request.model, request.prompt, request.provider, store.config.Currency, request.owner, store.config.LeaseTTL.Microseconds(), request.id)
	if err != nil {
		return false, err
	}
	reason := "开始诊断"
	if row.status == "waiting_approval" {
		reason = "审批已有决定，从保存的调用继续"
	}
	if row.status == "running" {
		reason = "租约过期，由新执行者从保存的步骤继续"
	}
	payload, _ := json.Marshal(map[string]string{"status": "running", "reason": reason})
	step, err := store.appendStep(ctx, tx, request.id, agent.Step{Kind: "system", Status: "running", Result: string(payload)})
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	store.publish(request.id, step)
	if store.config.Changed != nil {
		store.config.Changed(row.target, "running", 0)
	}
	return true, nil
}

// Renew 的条件更新不允许续活已经过期的租约；迟到者必须退出，不能自行复活。
func (store *Store) Renew(ctx context.Context, id string) (bool, error) {
	result, err := store.database.ExecContext(ctx, renewLeaseSQL, store.config.LeaseTTL.Microseconds(), id, llm.Lease(ctx))
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

// staleLeases 和接手锁住同一协调行，防止按旧快照结束刚续上执行权的任务。
func (store *Store) staleLeases(ctx context.Context) error {
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var guard int
	if err = tx.QueryRowContext(ctx, "SELECT id FROM scheduler_guard WHERE id=1 FOR UPDATE").Scan(&guard); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, staleLeasesSQL, -store.config.ResumeMaxAge.Microseconds())
	if err != nil {
		return err
	}
	type stale struct {
		id        string
		cancelled bool
	}
	var tasks []stale
	for rows.Next() {
		var item stale
		if err = rows.Scan(&item.id, &item.cancelled); err != nil {
			_ = rows.Close()
			return err
		}
		tasks = append(tasks, item)
	}
	readErr := rows.Err()
	_ = rows.Close()
	if readErr != nil {
		return readErr
	}
	var steps []agent.Step
	for _, task := range tasks {
		state, reason := "failed", "中断时间超过恢复期限，旧证据已过时，未继续诊断"
		if task.cancelled {
			state, reason = "cancelled", "用户取消"
		}
		if _, err = tx.ExecContext(ctx, finishStaleLeaseSQL, state, reason, task.id); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]string{"status": state, "reason": reason})
		step, err := store.appendStep(ctx, tx, task.id, agent.Step{Kind: "system", Status: state, Result: string(payload)})
		if err != nil {
			return err
		}
		steps = append(steps, step)
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	for index, task := range tasks {
		store.publish(task.id, steps[index])
	}
	return nil
}

// Resume 先补齐“已经记账但没保存回复步骤”的空窗，再返回完整历史。
func (store *Store) Resume(ctx context.Context, id string) ([]agent.Step, time.Duration, int64, error) {
	if err := store.recoverModelCalls(ctx, id); err != nil {
		return nil, 0, 0, err
	}
	var active, cost int64
	err := store.database.QueryRowContext(ctx, leasedActiveTimeSQL, id, llm.Lease(ctx)).Scan(&active)
	if err != nil {
		return nil, 0, 0, err
	}
	if err = store.database.QueryRowContext(ctx, "SELECT COALESCE(SUM(charged_micros),0) FROM llm_attempts WHERE diagnosis_id=?", id).Scan(&cost); err != nil {
		return nil, 0, 0, err
	}
	// 每页读取数量有限，按步骤号继续，历史再长也不能只恢复第一页。
	var all []agent.Step
	for after := 0; ; {
		steps, readErr := store.StepsAfter(ctx, id, after)
		if readErr != nil {
			return nil, 0, 0, readErr
		}
		all = append(all, steps...)
		if len(steps) < 100 {
			break
		}
		after = steps[len(steps)-1].Seq
	}
	return all, time.Duration(active) * time.Millisecond, cost, nil
}

const lockLeaseDiagnosisSQL = `SELECT
    target,
    status,
    COALESCE(lease_until <= UTC_TIMESTAMP(6), TRUE),
    cancel_requested,
    currency,
    COALESCE(lease_updated_at >= TIMESTAMPADD(MICROSECOND, ?, UTC_TIMESTAMP(6)), FALSE),
    ` + "`trigger`" + `,
    eval_expected_model,
    eval_profile,
    eval_provider,
    eval_account
FROM diagnoses
WHERE id = ?
    FOR
    UPDATE`

const occupiedTargetSlotsSQL = `SELECT
    COUNT(*),
    COALESCE(SUM(target = ?), 0)
FROM diagnoses
WHERE id <> ?
    AND (status = 'running'
         OR status = 'waiting_approval')`

const claimLeaseSQL = `UPDATE diagnoses
SET
    active_ms = active_ms + IF(status = 'running'
                               AND lease_updated_at IS NOT NULL, GREATEST(0, TIMESTAMPDIFF(MICROSECOND, lease_updated_at, LEAST(UTC_TIMESTAMP(6), lease_until)) DIV 1000), 0),
    model = IF(status = 'pending', ?, model),
    prompt_version = IF(status = 'pending', ?, prompt_version),
    provider = ?,
    currency = ?,
    started_at = COALESCE(started_at, UTC_TIMESTAMP(6)),
    execution_segments = IF(execution_segments = 0
                            AND status <> 'pending', 2, execution_segments + 1),
    status = 'running',
    lease_owner = ?,
    lease_until = TIMESTAMPADD(MICROSECOND, ?, UTC_TIMESTAMP(6)),
    lease_updated_at = UTC_TIMESTAMP(6)
WHERE id = ?`

const renewLeaseSQL = `UPDATE diagnoses
SET
    active_ms = active_ms + GREATEST(0, TIMESTAMPDIFF(MICROSECOND, lease_updated_at, UTC_TIMESTAMP(6)) DIV 1000),
    lease_updated_at = UTC_TIMESTAMP(6),
    lease_until = TIMESTAMPADD(MICROSECOND, ?, UTC_TIMESTAMP(6))
WHERE id = ?
    AND status = 'running'
    AND lease_owner = ?
    AND lease_until > UTC_TIMESTAMP(6)
    AND cancel_requested = FALSE`

const staleLeasesSQL = `SELECT
    id,
    cancel_requested
FROM diagnoses
WHERE ` + "`trigger`" + ` <> 'judge'
    AND status = 'running'
    AND (lease_until IS NULL
         OR lease_until <= UTC_TIMESTAMP(6))
    AND (cancel_requested = TRUE
         OR lease_updated_at IS NULL
         OR lease_updated_at < TIMESTAMPADD(MICROSECOND, ?, UTC_TIMESTAMP(6)))
    FOR
    UPDATE`

const finishStaleLeaseSQL = `UPDATE diagnoses
SET
    status = ?,
    error = ?,
    finished_at = UTC_TIMESTAMP(6),
    lease_owner = NULL,
    lease_until = NULL
WHERE id = ?`

const leasedActiveTimeSQL = `SELECT active_ms
FROM diagnoses
WHERE id = ?
    AND lease_owner = ?
    AND lease_until > UTC_TIMESTAMP(6)`
