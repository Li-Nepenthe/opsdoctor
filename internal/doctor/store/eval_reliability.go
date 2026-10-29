// 本文件：区分模型没能开始回答和已经回答却诊断失败的情况。
//
// 一般只有前一种基础设施故障可以补跑一次。另接受用户明确批准的特定校时
// 中断；这项许可必须有单独审计。不能只根据报告错误文字把推理失败改名豁免。
//
// 主要内容：EvalReliability 读取请求状态；infrastructureFailure 分类；
// PrepareEvalMakeup 为符合条件的原槽位生成唯一补跑槽位。
// 阅读建议：补跑沿用原始输入和故障时间，原失败和未知费用都保留。

package store

import (
	"context"
	"fmt"
)

// EvalReliability 返回是否因基础设施失败而一次有效模型响应也没有拿到。
func (store *Store) EvalReliability(ctx context.Context, id string) (bool, error) {
	// 只认评测任务的终态；普通诊断和执行中的中间状态不能变成“可补跑”。
	var state string
	var hasReport bool
	if err := store.database.QueryRowContext(ctx, "SELECT status,(report IS NOT NULL AND JSON_TYPE(report)<>'NULL') FROM diagnoses WHERE id=? AND `trigger`='eval'", id).Scan(&state, &hasReport); err != nil {
		return false, err
	}
	rows, err := store.database.QueryContext(ctx, "SELECT status FROM llm_attempts WHERE diagnosis_id=? ORDER BY created_at,id", id)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	// 读全所有尝试，不能只看最后一次断网，漏掉前面已经成功的模型回答。
	var statuses []string
	for rows.Next() {
		var status string
		if err = rows.Scan(&status); err != nil {
			return false, err
		}
		statuses = append(statuses, status)
	}
	if err = rows.Err(); err != nil {
		return false, err
	}
	if err = rows.Close(); err != nil {
		return false, err
	}
	if state == "cancelled" && !hasReport {
		return store.clockMakeupAllowed(ctx, id) // 用户批准的特定校时中断，不扩大一般网络豁免。
	}
	return infrastructureFailure(state, hasReport, statuses), nil
}

// infrastructureFailure 使用有限白名单，取消、余额不足和损坏的成功回复不能借机补跑。
func infrastructureFailure(state string, hasReport bool, statuses []string) bool {
	if state != "failed" || hasReport || len(statuses) == 0 {
		return false // 尚未发送、没有终态、已回答或人为取消，都不符合用户授权的补跑条件。
	}
	for _, status := range statuses {
		switch status {
		case "network_error", "network_timeout", "http_429", "http_500", "http_502", "http_503", "http_504":
			// 请求被限流或供应商暂时不可用，尚未得到有效的模型回答。
		default:
			return false // 包含一次 ok 就不再豁免；未知状态也不能擅自当成基础设施失败。
		}
	}
	return true
}

// PrepareEvalMakeup 只接受原槽位。唯一键使两次续跑命令取回同一份补跑安排。
func (store *Store) PrepareEvalMakeup(ctx context.Context, originalKey string) (EvalResult, error) {
	original, err := store.readEvalResult(ctx, originalKey)
	if err != nil {
		return EvalResult{}, err
	}
	if original.Attempt != 0 || original.RetryOf != "" || original.DiagnosisID == "" || len(original.Data) == 0 {
		return EvalResult{}, fmt.Errorf("只能为已保存结果的原槽位补跑一次")
	}
	allowed, err := store.EvalReliability(ctx, original.DiagnosisID)
	if err != nil {
		return EvalResult{}, err
	}
	if !allowed {
		return EvalResult{}, fmt.Errorf("这次失败不是未获得任何有效回答的基础设施失败，不能补跑")
	}
	key := original.Key + "/makeup"
	if len(key) > 128 {
		return EvalResult{}, fmt.Errorf("原槽位编号过长，无法附加补跑编号")
	}
	// 所有复制字段来自已保存的原槽位，调用方不能借补跑换模型输入或故障时间。
	_, err = store.database.ExecContext(ctx, "INSERT INTO eval_results(result_key,run_id,scenario,iteration,experiment_group,position,fault_started_at,input,attempt,retry_of,execution_profile) SELECT ?,original.run_id,original.scenario,original.iteration,original.experiment_group,original.position,original.fault_started_at,original.input,1,original.result_key,original.execution_profile FROM eval_results AS original WHERE original.result_key=? ON DUPLICATE KEY UPDATE result_key=eval_results.result_key", key, original.Key)
	if err != nil {
		return EvalResult{}, err
	}
	return store.readEvalResult(ctx, key)
}
