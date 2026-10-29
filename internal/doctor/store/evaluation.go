// 本文件：保存评测期间的调度边界和评测器创建的诊断。
//
// 告警仍然要接收，否则评测器看不到真实症状。暂停的是自动执行；捕获的通知保留
// 为已取消记录。评测器再用同一份通知创建各组任务，任务的工具组和模型在创建时固定。
//
// 主要内容：SetEvaluationMode 暂停或恢复自动调度；Evaluation 创建可重试的实验任务；
// EvaluationMode 供页面和命令行确认当前模式。
// 阅读建议：模式切换与调度领取共用协调锁，不能在检查空闲之后偷偷启动一条付费任务。

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"
)

// EvaluationMode 记录是谁暂停了自动诊断；必须由同一批评测负责恢复。
type EvaluationMode struct {
	Enabled bool   `json:"enabled"` // 进程退出不自动解除，避免重启意外花钱。
	RunID   string `json:"run_id"`  // 是批次标识，不包含场景答案。
}

// EvaluationInput 只含实验安排和捕获的症状，标准答案始终留在评测器一侧。
type EvaluationInput struct {
	Target         string          // 已接入的系统。
	RunID          string          // 当前拥有评测模式的批次。
	Key            string          // 批次、场景轮次和组的唯一编号，重试不能重复收费。
	Group          string          // E0–E3 决定工具菜单。
	ExpectedModel  string          // 启动时必须使用这个模型。
	FaultStartedAt time.Time       // 注入前保存的时间，所有配对组共用。
	Symptoms       json.RawMessage // 告警快照或例行检查说明，不含答案。
}

// EvaluationMode 不存在时返回关闭状态；数据库读取错误不能被当作“没在评测”。
func (store *Store) EvaluationMode(ctx context.Context, target string) (EvaluationMode, error) {
	var value EvaluationMode
	err := store.database.QueryRowContext(ctx, "SELECT enabled,run_id FROM evaluation_targets WHERE target=?", target).Scan(&value.Enabled, &value.RunID)
	if errors.Is(err, sql.ErrNoRows) {
		return value, nil
	}
	return value, err
}

// SetEvaluationMode 只在没有执行中或等待审批的任务时切换，防止模型配置跨过一条诊断。
func (store *Store) SetEvaluationMode(ctx context.Context, target, runID string, enabled bool) error {
	if !regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`).MatchString(target) || !regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`).MatchString(runID) {
		return fmt.Errorf("评测系统名或批次编号不合法")
	}
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var guard, busy int
	if err = tx.QueryRowContext(ctx, "SELECT id FROM scheduler_guard WHERE id=1 FOR UPDATE").Scan(&guard); err != nil {
		return err
	}
	var previous EvaluationMode
	err = tx.QueryRowContext(ctx, "SELECT enabled,run_id FROM evaluation_targets WHERE target=? FOR UPDATE", target).Scan(&previous.Enabled, &previous.RunID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if previous.Enabled && previous.RunID != runID {
		return fmt.Errorf("这个系统由另一批评测占用，请先完成或恢复原批次")
	}
	if previous.Enabled == enabled && previous.RunID == runID {
		return nil
	} // 同一批断点续跑无需重新切换。
	var registered string
	if enabled {
		if err = tx.QueryRowContext(ctx, "SELECT target FROM eval_runs WHERE id=?", runID).Scan(&registered); err != nil {
			return err
		}
		if registered != target {
			return fmt.Errorf("评测批次绑定了另一个系统")
		}
	}
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM diagnoses WHERE target=? AND (status IN ('running','waiting_approval') OR (`trigger`='eval' AND status='pending'))", target).Scan(&busy); err != nil {
		return err
	}
	if busy != 0 {
		return fmt.Errorf("系统还有执行中、等待审批或排队的评测任务，不能切换模式")
	}
	if !enabled {
		// 退出时还要等告警观察期结束，否则刚解除暂停就可能为上一场故障再次花钱。
		now := store.config.Now().UTC()
		if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM alert_groups WHERE target=? AND diagnosis_id IS NOT NULL AND ((closed_at IS NULL AND last_seen_at>?) OR recovery_until>?)", target, now.Add(-store.config.FaultSilence), now).Scan(&busy); err != nil {
			return err
		}
		if busy != 0 {
			return fmt.Errorf("故障仍开放或正在恢复观察，暂不能恢复自动诊断")
		}
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO evaluation_targets(target,run_id,enabled) VALUES(?,?,?) ON DUPLICATE KEY UPDATE run_id=?,enabled=?,updated_at=UTC_TIMESTAMP(6)", target, runID, enabled, runID, enabled); err != nil {
		return err
	}
	if enabled {
		// 已排队的旧告警也不能留到退出模式后付费执行；保留明确原因供页面说明。
		_, err = tx.ExecContext(ctx, "UPDATE diagnoses SET status='cancelled',error='进入评测模式，告警自动诊断已暂停',finished_at=UTC_TIMESTAMP(6) WHERE target=? AND `trigger`='alert' AND status='pending'", target)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Evaluation 用唯一键收回同一个任务；返回旧 ID 前核对配置，不能把不同实验误认成重试。
func (store *Store) Evaluation(ctx context.Context, value EvaluationInput) (string, error) {
	if !slices.Contains([]string{"E0", "E1", "E2", "E3"}, value.Group) || value.ExpectedModel == "" || len(value.ExpectedModel) > 128 || value.FaultStartedAt.IsZero() || len(value.Key) == 0 || len(value.Key) > 128 || len(value.Symptoms) > 65536 || !json.Valid(value.Symptoms) {
		return "", fmt.Errorf("评测配置、故障时间或症状不合法")
	}
	clean, err := store.config.Redactor.JSON(value.Symptoms)
	if err != nil {
		return "", err
	}
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var mode EvaluationMode
	var guard int
	if err = tx.QueryRowContext(ctx, "SELECT id FROM scheduler_guard WHERE id=1 FOR UPDATE").Scan(&guard); err != nil {
		return "", err
	}
	if err = tx.QueryRowContext(ctx, "SELECT enabled,run_id FROM evaluation_targets WHERE target=? FOR UPDATE", value.Target).Scan(&mode.Enabled, &mode.RunID); err != nil {
		return "", err
	}
	if !mode.Enabled || mode.RunID != value.RunID {
		return "", fmt.Errorf("请先为这批评测暂停系统的自动诊断")
	}
	id := newID()
	_, err = tx.ExecContext(ctx, "INSERT INTO diagnoses(id,target,`trigger`,status,title,alert_group_key,input,fault_started_at,eval_group,eval_expected_model,eval_key,eval_run_id) VALUES(?,?,'eval','pending',?,'',?,?,?,?,?,?) ON DUPLICATE KEY UPDATE id=id", id, value.Target, "评测 "+value.Group, clean, value.FaultStartedAt.UTC(), value.Group, value.ExpectedModel, value.Key, value.RunID)
	if err != nil {
		return "", err
	}
	var target, group, model, runID string
	var savedInput []byte
	var start time.Time
	if err = tx.QueryRowContext(ctx, "SELECT id,target,eval_group,eval_expected_model,fault_started_at,input,eval_run_id FROM diagnoses WHERE eval_key=?", value.Key).Scan(&id, &target, &group, &model, &start, &savedInput, &runID); err != nil {
		return "", err
	}
	// MySQL 的 JSON 会调整空白，解码后比较规范编码，不能靠原始字节判断同一份输入。
	if target != value.Target || group != value.Group || model != value.ExpectedModel || runID != value.RunID || !start.Equal(value.FaultStartedAt.UTC().Truncate(time.Microsecond)) || !sameJSON(savedInput, clean) {
		return "", fmt.Errorf("同一评测编号已有不同配置，拒绝覆盖或重新收费")
	}
	return id, tx.Commit()
}

// sameJSON 忽略数据库的 JSON 排版变化，但保留内容差异。
func sameJSON(left, right []byte) bool {
	var a, b any
	if json.Unmarshal(left, &a) != nil || json.Unmarshal(right, &b) != nil {
		return false
	}
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
