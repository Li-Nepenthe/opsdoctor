// 本文件：记录一轮故障执行到哪一步，让宿主机评测器安全续跑。
//
// 注入和恢复是数据库之外的动作，无法与 SQL 一起提交。先写“正在执行”，
// 再操作外部系统；重启后遇到不确定状态，先核对恢复文件，不能直接再注入。
//
// 主要内容：EvalTrial 取回原状态；AdvanceEvalTrial 条件推进；
// InterruptEval 追加暂停原因；SetEvalRunState 更新页面上的批次状态。
// 阅读建议：restored_at 由数据库取时，冷却等待以数据库的时间差为准。

package store

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

// EvalTrial 是一个场景的一次注入；五个实验组共享这份恢复进度。
type EvalTrial struct {
	RunID           string     `json:"run_id"`           // 已经冻结的批次。
	Scenario        string     `json:"scenario"`         // 不交给医生的实验标识。
	Iteration       int        `json:"iteration"`        // 不超过三轮。
	State           string     `json:"state"`            // pending 到 done 只向前走。
	FaultStartedAt  *time.Time `json:"fault_started_at"` // 对照本机恢复文件，避免串轮。
	RestoredAt      *time.Time `json:"restored_at"`      // 恢复确认后的数据库时间。
	CooldownSeconds int64      `json:"cooldown_seconds"` // 距离五分钟冷却结束还剩多久。
}

// EvalTrial 对相同位置只创建一次，返回原值而不是调用者猜测的状态。
func (store *Store) EvalTrial(ctx context.Context, runID, scenario string, iteration int) (EvalTrial, error) {
	value := EvalTrial{RunID: runID, Scenario: scenario, Iteration: iteration}
	if !regexp.MustCompile(`^S(0[1-9]|1[0-3])$`).MatchString(scenario) || iteration < 1 || iteration > 3 {
		return value, fmt.Errorf("实验轮次不合法")
	}
	_, err := store.database.ExecContext(ctx, "INSERT INTO eval_trials(run_id,scenario,iteration) VALUES(?,?,?) ON DUPLICATE KEY UPDATE run_id=run_id", runID, scenario, iteration)
	if err != nil {
		return value, err
	}
	err = store.database.QueryRowContext(ctx, "SELECT state,fault_started_at,restored_at,GREATEST(0,300-TIMESTAMPDIFF(SECOND,COALESCE(restored_at,UTC_TIMESTAMP(6)),UTC_TIMESTAMP(6))) FROM eval_trials WHERE run_id=? AND scenario=? AND iteration=?", runID, scenario, iteration).Scan(&value.State, &value.FaultStartedAt, &value.RestoredAt, &value.CooldownSeconds)
	return value, err
}

// trialTransition 只允许相邻进度。失败不能直接跳到 done，把漏跑伪装成完成。
func trialTransition(from, to string) bool {
	return map[string]string{"pending": "injecting", "injecting": "verifying", "verifying": "diagnosing", "diagnosing": "restoring", "restoring": "cooling", "cooling": "done"}[from] == to && to != ""
}

// AdvanceEvalTrial 用旧状态作条件，两个进程不能同时推进同一轮。
func (store *Store) AdvanceEvalTrial(ctx context.Context, value EvalTrial, next string, fault *time.Time) error {
	if !trialTransition(value.State, next) || (next == "verifying" && (fault == nil || fault.IsZero())) {
		return fmt.Errorf("实验轮次转换不合法")
	}
	// 故障开始时间只能在确认注入时写一次；恢复时间由数据库产生，不混用宿主机墙钟。
	result, err := store.database.ExecContext(ctx, "UPDATE eval_trials SET state=?,fault_started_at=COALESCE(fault_started_at,?),restored_at=IF(?='cooling',UTC_TIMESTAMP(6),restored_at),updated_at=UTC_TIMESTAMP(6) WHERE run_id=? AND scenario=? AND iteration=? AND state=?", next, fault, next, value.RunID, value.Scenario, value.Iteration, value.State)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count != 1 {
		return fmt.Errorf("实验轮次已被其他执行器推进，请重新读取")
	}
	return err
}

// InterruptEval 保存每次失败，不覆盖旧原因；内容仍先经过统一脱敏。
func (store *Store) InterruptEval(ctx context.Context, value EvalTrial, reason string) error {
	_, err := store.database.ExecContext(ctx, "INSERT INTO eval_interruptions(run_id,scenario,iteration,state,reason) VALUES(?,?,?,?,?)", value.RunID, value.Scenario, value.Iteration, value.State, store.config.Redactor.Text(reason))
	return err
}

// SetEvalRunState 只更新执行进度，不能清空 token 账本自己设置的暂停原因。
func (store *Store) SetEvalRunState(ctx context.Context, id, state string) error {
	if state != "running" && state != "interrupted" && state != "completed" {
		return fmt.Errorf("批次状态不合法")
	}
	_, err := store.database.ExecContext(ctx, "UPDATE eval_runs SET status=?,updated_at=UTC_TIMESTAMP(6) WHERE id=?", state, id)
	return err
}
