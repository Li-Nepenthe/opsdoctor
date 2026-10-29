// 本文件：把真实模型的费用预留与结算保存到医生数据库。
//
// 两个诊断同时运行时，分别检查“还剩多少钱”再发送会超支。因此先在一行账本上
// 原子扣出预留额，再发送 HTTP。拿到实际用量后退还差额；拿不到时保守保留。
//
// 主要内容：NewLedger 绑定账户与上限；Reserve 发送前记账；Settle 结算；Status 提供可见余额。
// 阅读建议：测试使用独立账户名；真实进程从配置绑定账户，重启不会重新获得额度。

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
)

// Ledger 保存不随诊断变化的累计账户，单位统一为百万分之一元。
type Ledger struct {
	store   *Store
	account string
	limit   int64
}

// NewLedger 创建空账本但绝不清零已有记录，配置降低上限也不会隐藏已花费用。
func (store *Store) NewLedger(ctx context.Context, account string, limit int64) (*Ledger, error) {
	if account == "" || len(account) > 64 || limit <= 0 || limit > 1_000_000_000_000 {
		return nil, fmt.Errorf("费用账户或总上限不合法")
	}
	_, err := store.database.ExecContext(ctx, "INSERT INTO llm_budget(account) VALUES(?) ON DUPLICATE KEY UPDATE account=account", account)
	if err != nil {
		return nil, err
	}
	return &Ledger{store: store, account: account, limit: limit}, nil
}

// Reserve 先锁预算再保存请求，两个动作一起提交后调用方才能联网。
func (ledger *Ledger) Reserve(ctx context.Context, value llm.Reservation) (string, error) {
	if value.MaximumMicros <= 0 || value.MaximumMicros > ledger.limit {
		return "", llm.ErrBudget
	}
	clean, err := ledger.store.config.Redactor.JSON(value.Request)
	if err != nil {
		return "", err
	}
	tx, err := ledger.store.database.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE llm_budget SET spent_micros=spent_micros+? WHERE account=? AND spent_micros<=? AND stopped_reason=''", value.MaximumMicros, ledger.account, ledger.limit-value.MaximumMicros)
	if err != nil {
		return "", err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if count != 1 {
		return "", llm.ErrBudget
	}
	// 账户行已锁住，同账户的两次预留不能同时读到旧的诊断费用。
	// 失败尝试和未结算请求也在 charged_micros 中，重启不能让它们从预算里消失。
	if value.DiagnosisLimitMicros > 0 {
		// 同一诊断即使在改账户后恢复，也只允许一份预算。锁诊断行使不同账户的预留串行。
		var diagnosis string
		if err = tx.QueryRowContext(ctx, "SELECT id FROM diagnoses WHERE id=? FOR UPDATE", value.DiagnosisID).Scan(&diagnosis); err != nil {
			return "", err
		}
		var spent int64
		if err = tx.QueryRowContext(ctx, "SELECT COALESCE(SUM(charged_micros),0) FROM llm_attempts WHERE diagnosis_id=?", value.DiagnosisID).Scan(&spent); err != nil {
			return "", err
		}
		if spent+value.MaximumMicros > value.DiagnosisLimitMicros {
			return "", llm.ErrDiagnosisBudget
		}
	}
	// 请求已经是实际发送的脱敏 JSON，审计内容不会比线上请求多一份敏感原文。
	id := newID()
	_, err = tx.ExecContext(ctx, "INSERT INTO llm_attempts(id,diagnosis_id,account,requested_model,request,maximum_micros,charged_micros,input_price_micros,cache_hit_price_micros,output_price_micros) VALUES(?,?,?,?,?,?,?,?,?,?)", id, value.DiagnosisID, ledger.account, value.Model, clean, value.MaximumMicros, value.MaximumMicros, value.Prices.InputMicros, value.Prices.CacheHitMicros, value.Prices.OutputMicros)
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

// Settle 可重复调用但只结算一次，避免重试数据库写入时重复退钱。
func (ledger *Ledger) Settle(ctx context.Context, id string, value llm.Settlement) error {
	if value.CostMicros < 0 || value.Usage.Input < 0 || value.Usage.Output < 0 || value.Usage.CacheHit < 0 {
		return fmt.Errorf("费用或 token 不能为负数")
	}
	if len(value.Response) == 0 {
		value.Response = json.RawMessage(`null`)
	}
	clean, err := ledger.store.config.Redactor.JSON(value.Response)
	if err != nil {
		return err
	}
	tx, err := ledger.store.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 和 Reserve 保持相同锁顺序：先账户、后尝试，避免并发结算发生相反顺序的等待。
	var spent, charged int64
	if err = tx.QueryRowContext(ctx, "SELECT spent_micros FROM llm_budget WHERE account=? FOR UPDATE", ledger.account).Scan(&spent); err != nil {
		return err
	}
	var settled sql.NullTime
	if err = tx.QueryRowContext(ctx, "SELECT charged_micros,settled_at FROM llm_attempts WHERE id=? AND account=? FOR UPDATE", id, ledger.account).Scan(&charged, &settled); err != nil {
		return err
	}
	if settled.Valid {
		return nil
	}
	actual := value.CostMicros
	if value.Unknown {
		actual = charged
	}
	// 即使供应商的计量与估算不一致，也必须把已发生的费用记全。额外保存停用原因，
	// 让重启后的调度器仍然停止付费，而不是只让当前一次诊断失败。
	overflow := actual > charged
	_, err = tx.ExecContext(ctx, "UPDATE llm_budget SET spent_micros=? WHERE account=?", spent-charged+actual, ledger.account)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE llm_attempts SET response=?,charged_micros=?,tokens_in=?,tokens_out=?,cache_hit_tokens=?,status=?,settled_at=? WHERE id=?", clean, actual, value.Usage.Input, value.Usage.Output, value.Usage.CacheHit, value.Status, ledger.store.config.Now().UTC(), id)
	if err != nil {
		return err
	}
	if overflow {
		_, err = tx.ExecContext(ctx, "UPDATE llm_budget SET stopped_reason=? WHERE account=?", "实际费用超过保守预留额，需人工核对后再启用", ledger.account)
		if err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if overflow {
		return fmt.Errorf("实际费用已记账，后续调用已停止：%w", llm.ErrBudget)
	}
	return nil
}

// Status 读的是整个真实账户，包含其他诊断预留但还没结算的钱。
func (ledger *Ledger) Status(ctx context.Context) (run.BudgetStatus, error) {
	value := run.BudgetStatus{LimitMicros: ledger.limit}
	err := ledger.store.database.QueryRowContext(ctx, "SELECT spent_micros,stopped_reason FROM llm_budget WHERE account=?", ledger.account).Scan(&value.SpentMicros, &value.Reason)
	if value.Reason != "" {
		value.Blocked = true
		value.ReasonCode = "accounting_guard"
	} else if value.SpentMicros >= value.LimitMicros {
		value.Blocked = true
		value.Reason = llm.ErrBudget.Error()
		value.ReasonCode = "total_limit"
	}
	return value, err
}
