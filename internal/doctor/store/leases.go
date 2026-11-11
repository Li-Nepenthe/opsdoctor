// 本文件：在数据库里分配和续租诊断的执行权。
//
// 租约像一张会过期的值班证。持有者定期续期；进程消失后，新进程等它过期才接手。
// 每次接手换一个随机编号，旧进程即使恢复运行，也不能冒充新的持有者。
//
// 主要内容：claimLease 分配名额；Renew 续约并累加执行时间；requireLease 拦住迟到写入；
// staleLeases 结束证据已经过时的任务；Resume 取回恢复所需的步骤和费用。
// 阅读建议：数据库时钟负责到期判断，避免两台机器的时钟差让租约被提前抢走。

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

	row, err := store.readClaim(ctx, tx, request)
	if err != nil {
		return false, err
	}
	if ok, err := decideClaim(row, store.config, request); !ok {
		return false, err
	}
	return store.commitClaim(ctx, tx, request, row)
}

func (store *Store) readClaim(ctx context.Context, tx *sql.Tx, request claimRequest) (claimRow, error) {
	var row claimRow
	var guard int
	if err := tx.QueryRowContext(ctx, "SELECT id FROM scheduler_guard WHERE id=1 FOR UPDATE").Scan(&guard); err != nil {
		return row, err
	}
	err := tx.QueryRowContext(ctx, "SELECT target,status,COALESCE(lease_until<=UTC_TIMESTAMP(6),TRUE),cancel_requested,currency,COALESCE(lease_updated_at>=TIMESTAMPADD(MICROSECOND,?,UTC_TIMESTAMP(6)),FALSE),`trigger`,eval_expected_model,eval_profile,eval_provider,eval_account FROM diagnoses WHERE id=? FOR UPDATE",
		-store.config.ResumeMaxAge.Microseconds(), request.id,
	).Scan(
		&row.target, &row.status, &row.available, &row.cancelled, &row.currency, &row.recent,
		&row.trigger, &row.expectedModel, &row.profile, &row.expectedProvider, &row.account,
	)
	if err != nil {
		return row, err
	}
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM evaluation_targets WHERE target=? AND enabled=TRUE)", row.target).Scan(&row.evaluating); err != nil {
		return row, err
	}
	// 此时名额计数仍为零。如果诊断行本身已不准领取，后续名额查询不会改变结论。
	if ok, err := decideClaim(row, store.config, request); !ok {
		return row, err
	}
	err = row.readSlots(ctx, tx, request.id)
	return row, err
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
	err := tx.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM(target=?),0) FROM diagnoses WHERE id<>? AND (status='running' OR status='waiting_approval')", row.target, id).Scan(&occupied, &row.sameTarget)
	if err != nil || row.sameTarget > 0 {
		return err
	}
	return tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM diagnoses WHERE id<>? AND status='running' AND lease_until>UTC_TIMESTAMP(6)", id).Scan(&row.active)
}

func decideClaim(row claimRow, config Config, request claimRequest) (bool, error) {
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
	case "running":
		// 即使调度器尚未清理，过时证据也不能用于接手。
		if !row.available || !row.recent {
			return false, nil
		}
	default:
		return false, nil
	}
	return row.undecided <= 0 && row.sameTarget <= 0 && row.active < 2, nil
}

func claimEvaluationAllowed(row claimRow, config Config, request claimRequest) (bool, error) {
	// 候选查询之后可能刚进入评测模式，领租约时再核对一次。
	if row.evaluating && row.trigger != "eval" {
		return false, nil
	}
	if row.trigger == "eval" && (!row.evaluating || row.expectedModel != request.model) {
		return false, fmt.Errorf("评测模式未开启或运行模型与实验配置不一致")
	}
	if row.profile != "" && (row.profile != config.ExecutionProfile || row.expectedProvider != request.provider ||
		row.account != config.BudgetAccount || row.currency != config.Currency) {
		return false, fmt.Errorf("评测绑定的执行配置、供应商、账户或币种不一致")
	}
	return true, nil
}

func (store *Store) commitClaim(ctx context.Context, tx *sql.Tx, request claimRequest, row claimRow) (bool, error) {
	// 中断到租约过期的时间也保守计入执行预算；接手后的等待不能重新拿五分钟。
	// 旧版本正在运行的记录还没有段数，接手时直接记成两段，不能误报完整耗时。
	_, err := tx.ExecContext(ctx, `UPDATE diagnoses SET
 active_ms=active_ms+IF(status='running' AND lease_updated_at IS NOT NULL,GREATEST(0,TIMESTAMPDIFF(MICROSECOND,lease_updated_at,LEAST(UTC_TIMESTAMP(6),lease_until)) DIV 1000),0),
 model=IF(status='pending',?,model),prompt_version=IF(status='pending',?,prompt_version),provider=?,
 currency=?,started_at=COALESCE(started_at,UTC_TIMESTAMP(6)),execution_segments=IF(execution_segments=0 AND status<>'pending',2,execution_segments+1),status='running',lease_owner=?,
 lease_until=TIMESTAMPADD(MICROSECOND,?,UTC_TIMESTAMP(6)),lease_updated_at=UTC_TIMESTAMP(6) WHERE id=?`, request.model, request.prompt, request.provider, store.config.Currency, request.owner, store.config.LeaseTTL.Microseconds(), request.id)
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
	result, err := store.database.ExecContext(ctx, `UPDATE diagnoses SET active_ms=active_ms+GREATEST(0,TIMESTAMPDIFF(MICROSECOND,lease_updated_at,UTC_TIMESTAMP(6)) DIV 1000),lease_updated_at=UTC_TIMESTAMP(6),lease_until=TIMESTAMPADD(MICROSECOND,?,UTC_TIMESTAMP(6)) WHERE id=? AND status='running' AND lease_owner=? AND lease_until>UTC_TIMESTAMP(6) AND cancel_requested=FALSE`, store.config.LeaseTTL.Microseconds(), id, llm.Lease(ctx))
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

// staleLeases 和接手共用协调行，查出过时任务后不会误把刚刚获得新租约的任务结束掉。
func (store *Store) staleLeases(ctx context.Context) error {
	// 清理和接手争同一把数据库锁，避免清理者依据过期快照结束刚接手的任务。
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var guard int
	if err = tx.QueryRowContext(ctx, "SELECT id FROM scheduler_guard WHERE id=1 FOR UPDATE").Scan(&guard); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "SELECT id,cancel_requested FROM diagnoses WHERE `trigger`<>'judge' AND status='running' AND (lease_until IS NULL OR lease_until<=UTC_TIMESTAMP(6)) AND (cancel_requested=TRUE OR lease_updated_at IS NULL OR lease_updated_at<TIMESTAMPADD(MICROSECOND,?,UTC_TIMESTAMP(6))) FOR UPDATE", -store.config.ResumeMaxAge.Microseconds())
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
		if _, err = tx.ExecContext(ctx, "UPDATE diagnoses SET status=?,error=?,finished_at=UTC_TIMESTAMP(6),lease_owner=NULL,lease_until=NULL WHERE id=?", state, reason, task.id); err != nil {
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
	err := store.database.QueryRowContext(ctx, "SELECT active_ms FROM diagnoses WHERE id=? AND lease_owner=? AND lease_until>UTC_TIMESTAMP(6)", id, llm.Lease(ctx)).Scan(&active)
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
