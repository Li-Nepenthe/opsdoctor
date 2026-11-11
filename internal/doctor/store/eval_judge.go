// 本文件：为评委固定评分输入、账户和最终结果。评委有独立的 judge 诊断，普通调度器不领取它，人民币评分费用因此能与美元诊断分账。
// 调用方持有评测锁再读取审计。若审计已进入 running，只从账本恢复，不重新发送。

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm/openai"
)

// EvalJudge 固定评委运行身份；不提供修改已经保存的评分输入的入口。
type EvalJudge struct {
	ID     string          `json:"id"`     // 独立审计归属，不是被评分医生的编号。
	Status string          `json:"status"` // pending、running 或终态。
	Input  json.RawMessage `json:"input"`  // 实际发给评委的数据，已脱敏。
}

// EvalJudgeInput 在创建评委审计时绑定结果槽位、评分规则、账户和原始输入。
type EvalJudgeInput struct {
	ResultKey     string
	Model         string
	Provider      string
	Account       string
	RubricVersion string
	Input         json.RawMessage
}

// PrepareEvalJudge 在同一事务关联审计记录，同一个结果槽位只会产生一份评分。
func (store *Store) PrepareEvalJudge(ctx context.Context, request EvalJudgeInput) (EvalJudge, error) {
	var value EvalJudge
	if request.Model == "" || request.Account == "" || request.RubricVersion == "" || (request.Provider != "mock" && request.Provider != "openai") || !json.Valid(request.Input) || len(request.Input) > 240<<10 {
		return value, fmt.Errorf("评分配置或输入不合法")
	}
	clean, err := store.config.Redactor.JSON(request.Input)
	if err != nil {
		return value, err
	}
	// 先锁结果行再创建审计；重复请求只能看见同一个 judge_id。
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return value, err
	}
	defer tx.Rollback()
	var target, runID string
	var finished, graded bool
	if err = tx.QueryRowContext(ctx, "SELECT r.run_id,e.target,COALESCE(r.judge_id,''),r.result IS NOT NULL,r.judgement IS NOT NULL FROM eval_results r JOIN eval_runs e ON e.id=r.run_id WHERE r.result_key=? FOR UPDATE", request.ResultKey).Scan(&runID, &target, &value.ID, &finished, &graded); err != nil {
		return value, err
	}
	if !finished {
		return value, fmt.Errorf("诊断未完成，不能开始评分")
	}
	if value.ID == "" {
		value.ID = newID()
		_, err = tx.ExecContext(ctx, "INSERT INTO diagnoses(id,target,`trigger`,status,title,alert_group_key,input,model,provider,prompt_version,eval_account,currency,eval_run_id) VALUES(?,?,'judge','pending','评委审阅','',?,?,?,?,?,'CNY',?)", value.ID, target, clean, request.Model, request.Provider, request.RubricVersion, request.Account, runID)
		if err != nil {
			return value, err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE eval_results SET judge_id=? WHERE result_key=?", value.ID, request.ResultKey); err != nil {
			return value, err
		}
	}
	var saved struct{ model, provider, account, version string }
	if err = tx.QueryRowContext(ctx, "SELECT status,input,model,provider,eval_account,prompt_version FROM diagnoses WHERE id=?", value.ID).Scan(
		&value.Status,
		&value.Input,
		&saved.model,
		&saved.provider,
		&saved.account,
		&saved.version,
	); err != nil {
		return value, err
	}
	if !sameJSON(value.Input, clean) || saved.model != request.Model || saved.provider != request.Provider || saved.account != request.Account || saved.version != request.RubricVersion {
		return value, fmt.Errorf("评分已绑定另一份输入、模型、版本或账户")
	}
	return value, tx.Commit()
}

// StartEvalJudge 先保存 running，再允许客户端预留费用；崩溃后看到 running 不再重发。
func (store *Store) StartEvalJudge(ctx context.Context, id string) (bool, error) {
	var trigger string
	if err := store.database.QueryRowContext(ctx, "SELECT `trigger` FROM diagnoses WHERE id=?", id).Scan(&trigger); err != nil {
		return false, err
	}
	if trigger != "judge" {
		return false, fmt.Errorf("只允许启动评委审计记录")
	}
	return store.changeState(ctx, id, stateChange{
		from: "pending", to: "running", reason: "评测器开始评分",
		fields: "started_at=UTC_TIMESTAMP(6),execution_segments=1",
	})
}

// SavedEvalJudgeResponse 保留未结算预留，只有成功存入账本的回答可以恢复为评分材料。
func (store *Store) SavedEvalJudgeResponse(ctx context.Context, id string) (llm.Response, error) {
	var result llm.Response
	// 锁由调用者持有，旧进程已不再负责这次评分；未结算金额不退回。
	if _, err := store.database.ExecContext(ctx, "UPDATE llm_attempts SET status='interrupted',settled_at=UTC_TIMESTAMP(6) WHERE diagnosis_id=? AND settled_at IS NULL", id); err != nil {
		return result, err
	}
	rows, err := store.database.QueryContext(ctx, "SELECT id,requested_model,response,status,charged_micros FROM llm_attempts WHERE diagnosis_id=? ORDER BY created_at,id", id)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var attempt, model, status string
		var raw []byte
		var cost int64
		if err = rows.Scan(&attempt, &model, &raw, &status, &cost); err != nil {
			return result, err
		}
		result.AttemptIDs = append(result.AttemptIDs, attempt)
		result.CostMicros += cost
		if status != "ok" {
			continue
		}
		answer, err := openai.DecodeSavedResponse(raw, model)
		if err != nil {
			return result, err
		}
		result.Model, result.Message, result.Usage, result.UsageKnown = answer.Model, answer.Message, answer.Usage, true
	}
	return result, rows.Err()
}

// EvalJudgeResult 将评分和模型用量记入同一份审计。跨进程恢复时 Elapsed 为空。
type EvalJudgeResult struct {
	ResultKey string
	JudgeID   string
	Judgement json.RawMessage
	Response  llm.Response
	Elapsed   *time.Duration
	Problem   string
}

// FinishEvalJudge 同时保存评分和终态；失败、费用、原回复都保留，不允许换成更好的一次。
func (store *Store) FinishEvalJudge(ctx context.Context, result EvalJudgeResult) error {
	if !json.Valid(result.Judgement) || len(result.Judgement) > 65536 {
		return fmt.Errorf("评分结果不合法")
	}
	clean, err := store.config.Redactor.JSON(result.Judgement)
	if err != nil {
		return err
	}
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var previous []byte
	var savedID string
	if err = tx.QueryRowContext(ctx, "SELECT judge_id,judgement FROM eval_results WHERE result_key=? FOR UPDATE", result.ResultKey).Scan(&savedID, &previous); err != nil {
		return err
	}
	if savedID != result.JudgeID {
		return fmt.Errorf("评分与结果槽位不一致")
	}
	if len(previous) > 0 {
		if !sameJSON(previous, clean) {
			return fmt.Errorf("评分已经保存，不能覆盖")
		}
		return nil
	}
	status := "succeeded"
	if result.Problem != "" {
		status = "failed"
	}
	var duration any
	if result.Elapsed != nil {
		duration = max(0, result.Elapsed.Milliseconds())
	}
	// 跨进程恢复缺少完整单调时钟，时长保留 NULL；不拿当前处理时间冒充总时长。
	changed, err := tx.ExecContext(ctx, "UPDATE diagnoses SET status=?,report=?,error=?,tokens_in=?,tokens_out=?,cache_hit_tokens=?,cost_micros=?,duration_ms=?,finished_at=UTC_TIMESTAMP(6) WHERE id=? AND `trigger`='judge' AND status='running'", status, clean, store.config.Redactor.Text(result.Problem), result.Response.Usage.Input, result.Response.Usage.Output, result.Response.Usage.CacheHit, result.Response.CostMicros, duration, result.JudgeID)
	if err != nil {
		return err
	}
	count, err := changed.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("评分审计已被取消或不在运行状态")
	}
	if _, err = tx.ExecContext(ctx, "UPDATE eval_results SET judgement=? WHERE result_key=?", clean, result.ResultKey); err != nil {
		return err
	}
	return tx.Commit()
}
