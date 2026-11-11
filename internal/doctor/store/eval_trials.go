// 本文件：注入、恢复是数据库外的操作。先在数据库记录要执行的动作，重启后核对本机恢复文件，才知道是否还需注入。
// 恢复时刻和剩余冷却时间由数据库计时，免受宿主机时钟变化影响。

package store

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// EvalTrial 是一个场景的一次注入；五个实验组共享这份恢复进度。
type EvalTrial struct {
	RunID           string     `json:"run_id"`
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
	if !validEvalTrial(runID, scenario, iteration) {
		return value, fmt.Errorf("实验轮次不合法")
	}
	_, err := store.database.ExecContext(ctx, ensureEvalTrialSQL, runID, scenario, iteration)
	if err != nil {
		return value, err
	}
	err = store.database.QueryRowContext(ctx, readEvalTrialSQL, runID, scenario, iteration).Scan(&value.State, &value.FaultStartedAt, &value.RestoredAt, &value.CooldownSeconds)
	return value, err
}

// validEvalTrial 保留原 shop 编号；只有阶段 9 独立批次才允许外部系统的 OT 编号。
func validEvalTrial(runID, scenario string, iteration int) bool {
	if iteration < 1 || iteration > 3 {
		return false
	}
	if regexp.MustCompile(`^S(0[1-9]|1[0-3])$`).MatchString(scenario) {
		return true
	}
	return strings.HasPrefix(runID, "phase09-") && regexp.MustCompile(`^OT0[1-8]$`).MatchString(scenario)
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
	// Go 可以保存纳秒，DATETIME(6) 只能保存微秒。先主动截断，避免 MySQL
	// 四舍五入后比恢复文件晚一微秒，让同一次注入被误认成另一轮。
	var persistedFault *time.Time
	if fault != nil {
		value := fault.UTC().Truncate(time.Microsecond)
		persistedFault = &value
	}
	result, err := store.database.ExecContext(ctx, advanceEvalTrialSQL, next, persistedFault, next, value.RunID, value.Scenario, value.Iteration, value.State)
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
	_, err := store.database.ExecContext(ctx, interruptEvalTrialSQL, value.RunID, value.Scenario, value.Iteration, value.State, store.config.Redactor.Text(reason))
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

const ensureEvalTrialSQL = `INSERT INTO eval_trials (
    run_id,
    scenario,
    iteration
)
VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE run_id = run_id`

const readEvalTrialSQL = `SELECT
    state,
    fault_started_at,
    restored_at,
    GREATEST(0, 300 - TIMESTAMPDIFF(SECOND, COALESCE(restored_at, UTC_TIMESTAMP(6)), UTC_TIMESTAMP(6)))
FROM eval_trials
WHERE run_id = ?
    AND scenario = ?
    AND iteration = ?`

const advanceEvalTrialSQL = `UPDATE eval_trials
SET
    state = ?,
    fault_started_at = COALESCE(fault_started_at, ?),
    restored_at = IF(? = 'cooling', UTC_TIMESTAMP(6), restored_at),
    updated_at = UTC_TIMESTAMP(6)
WHERE run_id = ?
    AND scenario = ?
    AND iteration = ?
    AND state = ?`

const interruptEvalTrialSQL = `INSERT INTO eval_interruptions (
    run_id,
    scenario,
    iteration,
    state,
    reason
)
VALUES (?, ?, ?, ?, ?)`
