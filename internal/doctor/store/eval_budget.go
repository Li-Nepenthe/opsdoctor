// 本文件：在模型费用记账的同一个事务里限制整批评测的 token 消耗。
//
// 只在命令行进程里数 token 不够：进程崩溃以后，医生可能仍在完成请求。这里让
// 每次真实发送先在数据库预留。已知用量才退差额，未知结果保留，重启不清零。
//
// 主要内容：CreateEvalRun 固定批次配置；EvalRun 读取额度；reserveEvalTokens 和
// settleEvalTokens 跟随费用事务一起提交或回滚。
// 阅读建议：token 不是钱，美元和人民币仍由原有的两个费用账户分别限制。

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// EvalRun 是批次的公开进度，配置只保存版本与运行安排，不保存 Key。
type EvalRun struct {
	ID            string          `json:"id"`             // 操作者指定的稳定批次编号，续跑沿用。
	Target        string          `json:"target"`         // 评测的唯一系统。
	Config        json.RawMessage `json:"config"`         // 创建后不允许悄悄换组、模型或版本。
	Status        string          `json:"status"`         // preparing、running、completed、interrupted。
	TokenLimit    int64           `json:"token_limit"`    // 输入与输出合计，缓存命中也计入。
	TokensUsed    int64           `json:"tokens_used"`    // 已结算量加未确认的预留量。
	StoppedReason string          `json:"stopped_reason"` // 超过估计上界时留下核对原因。
}

// CreateEvalRun 相同参数可重复调用；更改配置要用新批次，不能覆盖之前的失败记录。
func (store *Store) CreateEvalRun(ctx context.Context, value EvalRun) error {
	// 重试只允许取回原配置，不能借“续跑”提高额度或换一份未审阅手册。
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`).MatchString(value.ID) || value.TokenLimit < 1 || value.TokenLimit > 1_000_000_000 || len(value.Config) > 65536 || !json.Valid(value.Config) {
		return fmt.Errorf("评测批次编号、配置或 token 上限不合法")
	}
	clean, err := store.config.Redactor.JSON(value.Config)
	if err != nil {
		return err
	}
	_, err = store.database.ExecContext(ctx, "INSERT INTO eval_runs(id,target,config,token_limit) VALUES(?,?,?,?) ON DUPLICATE KEY UPDATE id=id", value.ID, value.Target, clean, value.TokenLimit)
	if err != nil {
		return err
	}
	saved, err := store.EvalRun(ctx, value.ID)
	if err != nil {
		return err
	}
	if saved.Target != value.Target || saved.TokenLimit != value.TokenLimit || !sameJSON(saved.Config, clean) {
		return fmt.Errorf("已有评测批次配置不同，不能覆盖，请核对版本或创建新批次")
	}
	return nil
}

// EvalRun 只读这一批的 token 状态；费用必须另查对应币种的 Ledger。
func (store *Store) EvalRun(ctx context.Context, id string) (EvalRun, error) {
	var value EvalRun
	err := store.database.QueryRowContext(ctx, "SELECT id,target,config,status,token_limit,tokens_used,stopped_reason FROM eval_runs WHERE id=?", id).Scan(&value.ID, &value.Target, &value.Config, &value.Status, &value.TokenLimit, &value.TokensUsed, &value.StoppedReason)
	return value, err
}

// reserveEvalTokens 由 Reserve 在插入费用尝试后调用；任何一步失败，整份费用预留也回滚。
func reserveEvalTokens(ctx context.Context, tx *sql.Tx, attempt string, value llm.Reservation) error {
	var runID sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT eval_run_id FROM diagnoses WHERE id=?", value.DiagnosisID).Scan(&runID); err != nil {
		return err
	}
	if !runID.Valid {
		return nil
	} // 普通诊断没有批次 token 账户，仍受 Agent 和费用上限保护。
	if value.MaximumTokens <= 0 || value.MaximumTokens > 1_000_000_000 {
		return fmt.Errorf("评测请求缺少 token 预留量")
	}
	changed, err := tx.ExecContext(ctx, "UPDATE eval_runs SET tokens_used=tokens_used+?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND tokens_used+?<=token_limit AND stopped_reason=''", value.MaximumTokens, runID.String, value.MaximumTokens)
	if err != nil {
		return err
	}
	count, err := changed.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return llm.ErrEvaluationBudget
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO eval_attempts(attempt_id,run_id,maximum_tokens,charged_tokens) VALUES(?,?,?,?)", attempt, runID.String, value.MaximumTokens, value.MaximumTokens)
	return err
}

// settleEvalTokens 与费用结算共用“只结算一次”检查；未知量不退，已知缓存 token 不重复相加。
func settleEvalTokens(ctx context.Context, tx *sql.Tx, attempt string, value llm.Settlement) (bool, error) {
	// 普通诊断没有对应行；只有“查无此行”可以略过，数据库错误必须让整个结算回滚。
	var runID string
	var charged int64
	err := tx.QueryRowContext(ctx, "SELECT run_id,charged_tokens FROM eval_attempts WHERE attempt_id=?", attempt).Scan(&runID, &charged)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	actual := value.Usage.Input + value.Usage.Output
	if value.Unknown {
		actual = charged
	}
	reason := ""
	if actual > charged {
		reason = "实际 token 超过预留上界，请核对后再继续评测"
	}
	if _, err = tx.ExecContext(ctx, "UPDATE eval_runs SET tokens_used=tokens_used-?+?,stopped_reason=IF(?='',stopped_reason,?),updated_at=UTC_TIMESTAMP(6) WHERE id=?", charged, actual, reason, reason, runID); err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, "UPDATE eval_attempts SET charged_tokens=? WHERE attempt_id=?", actual, attempt)
	return actual > charged, err
}
