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

// claimLease 用一行短事务锁同时检查全局两个名额、系统一个名额，防止两进程各自超配。
func (store *Store) claimLease(ctx context.Context, id, model, prompt, provider string) (bool, error) {
	owner := llm.Lease(ctx)
	if owner == "" {
		return false, fmt.Errorf("开始诊断缺少租约编号")
	}
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var guard int
	if err = tx.QueryRowContext(ctx, "SELECT id FROM scheduler_guard WHERE id=1 FOR UPDATE").Scan(&guard); err != nil {
		return false, err
	}
	var target, status, currency, trigger, expectedModel string
	var available, cancelled, recent bool
	if err = tx.QueryRowContext(ctx, "SELECT target,status,COALESCE(lease_until<=UTC_TIMESTAMP(6),TRUE),cancel_requested,currency,COALESCE(lease_updated_at>=TIMESTAMPADD(MICROSECOND,?,UTC_TIMESTAMP(6)),FALSE),`trigger`,eval_expected_model FROM diagnoses WHERE id=? FOR UPDATE", -store.config.ResumeMaxAge.Microseconds(), id).Scan(&target, &status, &available, &cancelled, &currency, &recent, &trigger, &expectedModel); err != nil {
		return false, err
	}
	var evaluating bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM evaluation_targets WHERE target=? AND enabled=TRUE)", target).Scan(&evaluating); err != nil {
		return false, err
	}
	if evaluating && trigger != "eval" {
		return false, nil // 候选查询之后可能刚进入评测模式，领租约时必须再检查一次。
	}
	if trigger == "eval" && (!evaluating || expectedModel != model) {
		return false, fmt.Errorf("评测模式未开启或运行模型与实验配置不一致")
	}
	if status != "pending" && currency != store.config.Currency {
		return false, fmt.Errorf("不能跨币种恢复诊断，请保持原账户或取消后重新发起")
	}
	if cancelled || (status != "pending" && status != "running" && status != "waiting_approval") || (status == "running" && !available) {
		return false, nil
	}
	// 即使调用方漏了 Recover，已经过时的 running 也不能直接接手。
	if status == "running" && !recent {
		return false, nil
	}
	if status == "waiting_approval" {
		var undecided int
		if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM approvals WHERE diagnosis_id=? AND status='pending'", id).Scan(&undecided); err != nil {
			return false, err
		}
		if undecided > 0 {
			return false, nil
		}
	}
	var global, same int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM(target=?),0) FROM diagnoses WHERE id<>? AND (status='running' OR status='waiting_approval')", target, id).Scan(&global, &same); err != nil {
		return false, err
	}
	// 等待审批仍占系统名额，避免新诊断与未决写操作互相打架；不占模型执行的全局名额。
	if same > 0 {
		return false, nil
	}
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM diagnoses WHERE id<>? AND status='running' AND lease_until>UTC_TIMESTAMP(6)", id).Scan(&global); err != nil {
		return false, err
	}
	if global >= 2 {
		return false, nil
	}
	// 中断到租约过期的时间也保守计入执行预算；接手后的等待不能重新拿五分钟。
	// 旧版本正在运行的记录还没有段数，接手时直接记成两段，不能误报完整耗时。
	_, err = tx.ExecContext(ctx, `UPDATE diagnoses SET
 active_ms=active_ms+IF(status='running' AND lease_updated_at IS NOT NULL,GREATEST(0,TIMESTAMPDIFF(MICROSECOND,lease_updated_at,LEAST(UTC_TIMESTAMP(6),lease_until)) DIV 1000),0),
 model=IF(status='pending',?,model),prompt_version=IF(status='pending',?,prompt_version),provider=?,
 currency=?,started_at=COALESCE(started_at,UTC_TIMESTAMP(6)),execution_segments=IF(execution_segments=0 AND status<>'pending',2,execution_segments+1),status='running',lease_owner=?,
 lease_until=TIMESTAMPADD(MICROSECOND,?,UTC_TIMESTAMP(6)),lease_updated_at=UTC_TIMESTAMP(6) WHERE id=?`, model, prompt, provider, store.config.Currency, owner, store.config.LeaseTTL.Microseconds(), id)
	if err != nil {
		return false, err
	}
	reason := "开始诊断"
	if status == "waiting_approval" {
		reason = "审批已有决定，从保存的调用继续"
	}
	if status == "running" {
		reason = "租约过期，由新执行者从保存的步骤继续"
	}
	payload, _ := json.Marshal(map[string]string{"status": "running", "reason": reason})
	step, err := store.appendStep(ctx, tx, id, agent.Step{Kind: "system", Status: "running", Result: string(payload)})
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	store.publish(id, step)
	if store.config.Changed != nil {
		store.config.Changed(target, "running", 0)
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
	rows, err := tx.QueryContext(ctx, "SELECT id,cancel_requested FROM diagnoses WHERE status='running' AND (lease_until IS NULL OR lease_until<=UTC_TIMESTAMP(6)) AND (cancel_requested=TRUE OR lease_updated_at IS NULL OR lease_updated_at<TIMESTAMPADD(MICROSECOND,?,UTC_TIMESTAMP(6))) FOR UPDATE", -store.config.ResumeMaxAge.Microseconds())
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
