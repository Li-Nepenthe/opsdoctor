// 本文件：保存用户批准的流量守卫修订，并重启尚未创建任何模型槽位的中断轮次。
//
// 主要内容：AmendEvalBaseline 将旧轮次、版本和授权原子保存；baselineVersions 读取后继版本。
// 阅读建议：普通状态转换仍不能回退。这个入口必须对应已有的基线中断，而且只能用一次。

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AmendEvalBaseline 只供已获用户授权的宿主机命令调用，不开放给医生或模型。
func (store *Store) AmendEvalBaseline(ctx context.Context, trial EvalTrial, interruption int64, reason string, original, replacement json.RawMessage) error {
	if interruption < 1 || len(reason) < 10 || len(reason) > 2000 || !json.Valid(original) || !json.Valid(replacement) || sameJSON(original, replacement) {
		return fmt.Errorf("需要具体授权、中断编号和不同的新旧版本")
	}
	clean := store.config.Redactor.Text(reason)
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 锁住批次，使并发重试不能在同一个旧轮次上各自执行一次重置。
	var config []byte
	if err = tx.QueryRowContext(ctx, "SELECT config FROM eval_runs WHERE id=? FOR UPDATE", trial.RunID).Scan(&config); err != nil {
		return err
	}
	var before, after []byte
	var priorReason string
	var priorInterruption int64
	err = tx.QueryRowContext(ctx, "SELECT original_versions,replacement_versions,reason,interruption_id FROM eval_baseline_amendments WHERE run_id=?", trial.RunID).Scan(&before, &after, &priorReason, &priorInterruption)
	if err == nil {
		if !sameJSON(before, original) || !sameJSON(after, replacement) || priorReason != clean || priorInterruption != interruption {
			return fmt.Errorf("基线批准记录已存在，拒绝改写")
		}
		// 命令重试只确认同一记录，不再次把已经开始的新注入退回 pending。
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	// 数据库再次核对版本链，不能仅凭调用者给出的两个JSON就批准新版本。
	var frozen struct {
		Versions json.RawMessage `json:"versions"`
	}
	if json.Unmarshal(config, &frozen) != nil || len(frozen.Versions) == 0 {
		return fmt.Errorf("批次缺少原冻结版本")
	}
	effective := frozen.Versions
	var clockBefore, clockAfter []byte
	err = tx.QueryRowContext(ctx, "SELECT original_versions,replacement_versions FROM eval_clock_amendments WHERE run_id=?", trial.RunID).Scan(&clockBefore, &clockAfter)
	if err == nil {
		if !sameJSON(clockBefore, effective) {
			return fmt.Errorf("校时批准与原冻结版本不符")
		}
		effective = clockAfter
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if !sameJSON(effective, original) {
		return fmt.Errorf("基线批准的父版本不是当前有效版本")
	}
	var interrupted EvalTrial
	var problem string
	var interruptedAt time.Time
	err = tx.QueryRowContext(ctx, "SELECT scenario,iteration,state,reason,created_at FROM eval_interruptions WHERE id=? AND run_id=?", interruption, trial.RunID).Scan(&interrupted.Scenario, &interrupted.Iteration, &interrupted.State, &problem, &interruptedAt)
	if err != nil {
		return err
	}
	if interrupted.Scenario != trial.Scenario || interrupted.Iteration != trial.Iteration || interrupted.State != "diagnosing" || !strings.HasPrefix(problem, "评测基线要求") {
		return fmt.Errorf("不是已批准轮次的诊断前基线中断")
	}
	current := EvalTrial{RunID: trial.RunID, Scenario: trial.Scenario, Iteration: trial.Iteration}
	err = tx.QueryRowContext(ctx, "SELECT state,fault_started_at,restored_at FROM eval_trials WHERE run_id=? AND scenario=? AND iteration=? FOR UPDATE", trial.RunID, trial.Scenario, trial.Iteration).Scan(&current.State, &current.FaultStartedAt, &current.RestoredAt)
	if err != nil {
		return err
	}
	if current.State != "diagnosing" || current.FaultStartedAt == nil {
		return fmt.Errorf("中断轮次已改变，拒绝重新注入")
	}
	if interruptedAt.Before(*current.FaultStartedAt) {
		return fmt.Errorf("中断早于当前注入，不能用旧中断重启新故障")
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM eval_results WHERE run_id=? AND scenario=? AND iteration=?", trial.RunID, trial.Scenario, trial.Iteration).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("轮次已有模型槽位，不能用基线修订重做结果")
	}
	// 连尚未关联回槽位的孤立诊断也不接受，避免网络断开后误以为模型没有启动。
	prefix := fmt.Sprintf("%s/%s/%d/", trial.RunID, trial.Scenario, trial.Iteration)
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM diagnoses WHERE eval_run_id=? AND LEFT(eval_key,?)=?", trial.RunID, len(prefix), prefix).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("轮次已有诊断，不能重新注入")
	}
	// 保留整个旧轮次，包括原故障时间。只有审计写入成功，才更新当前执行检查点。
	raw, err := json.Marshal(current)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO eval_baseline_amendments(run_id,original_versions,replacement_versions,original_trial,interruption_id,reason) VALUES(?,?,?,?,?,?)", trial.RunID, original, replacement, raw, interruption, clean); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE eval_trials SET state='pending',fault_started_at=NULL,restored_at=NULL,updated_at=UTC_TIMESTAMP(6) WHERE run_id=? AND scenario=? AND iteration=? AND state='diagnosing'", trial.RunID, trial.Scenario, trial.Iteration); err != nil {
		return err
	}
	return tx.Commit()
}

// baselineVersions 接在旧校时修订之后，旧两份版本记录仍各自存在，不能跳过链条。
func (store *Store) baselineVersions(ctx context.Context, runID string, previous json.RawMessage) (json.RawMessage, error) {
	var before, after []byte
	err := store.database.QueryRowContext(ctx, "SELECT original_versions,replacement_versions FROM eval_baseline_amendments WHERE run_id=?", runID).Scan(&before, &after)
	if errors.Is(err, sql.ErrNoRows) {
		return previous, nil
	}
	if err != nil {
		return nil, err
	}
	if !sameJSON(before, previous) {
		return nil, fmt.Errorf("基线批准与前一版本不符")
	}
	return after, nil
}

// EvalBaselineParent 让同一批准命令可重试；已有记录时不能把新版本再次当作旧版本批准。
func (store *Store) EvalBaselineParent(ctx context.Context, runID string, original json.RawMessage) (json.RawMessage, error) {
	var parent []byte
	err := store.database.QueryRowContext(ctx, "SELECT original_versions FROM eval_baseline_amendments WHERE run_id=?", runID).Scan(&parent)
	if err == nil {
		return parent, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return store.EvalVersions(ctx, runID, original)
}
