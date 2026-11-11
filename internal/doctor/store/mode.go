// 本文件：把评测期间的调度、告警和审批规则放在一起。普通任务暂缓领取，告警照常落库，写操作的提议和拒绝原因都留下。
// 候选查询只筛选一次；领取租约时还会在锁内重新读取模式，防止两次操作之间切换系统。

package store

import (
	"context"
	"database/sql"
	"fmt"
)

// pendingCandidatesSQL 保留每个系统排在最前的候选，由数据库在同一次查询内排除评测中系统的普通诊断。
const pendingCandidatesSQL = "SELECT id,target,input,created_at,COALESCE(fault_started_at,created_at),eval_group,eval_expected_model,eval_profile FROM (SELECT id,target,input,created_at,fault_started_at,eval_group,eval_expected_model,eval_profile,ROW_NUMBER() OVER (PARTITION BY target ORDER BY CASE status WHEN 'running' THEN 0 WHEN 'waiting_approval' THEN 1 ELSE 2 END,created_at,id) AS position_in_target FROM diagnoses WHERE `trigger`<>'judge' AND (status='pending' OR (status='running' AND lease_until<=UTC_TIMESTAMP(6)) OR (status='waiting_approval' AND NOT EXISTS(SELECT 1 FROM approvals WHERE diagnosis_id=diagnoses.id AND status='pending'))) AND (`trigger`='eval' OR NOT EXISTS(SELECT 1 FROM evaluation_targets WHERE target=diagnoses.target AND enabled=TRUE))) AS candidates WHERE position_in_target=1 ORDER BY created_at,id LIMIT 128"

func evaluationEnabled(ctx context.Context, tx *sql.Tx, target string) (bool, error) {
	var enabled bool
	err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM evaluation_targets WHERE target=? AND enabled=TRUE)", target).Scan(&enabled)
	return enabled, err
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

type alertDisposition struct {
	status     string
	reason     string
	suppressed bool
}

// alertForMode 在原告警事务里决定是否调度。评测期间仍保留通知和故障开放期，但这条诊断直接终止。
func alertForMode(ctx context.Context, tx *sql.Tx, target string) (alertDisposition, error) {
	enabled, err := evaluationEnabled(ctx, tx, target)
	if err != nil {
		return alertDisposition{}, err
	}
	if enabled {
		return alertDisposition{status: "cancelled", reason: "评测期间仅记录告警，自动诊断已暂停", suppressed: true}, nil
	}
	return alertDisposition{status: "pending"}, nil
}

func proposalForMode(ctx context.Context, tx *sql.Tx, trigger, id string) (string, string, error) {
	state, message := "pending", "等待人工审批，尚未执行"
	if trigger == "eval" {
		// 评测要观察原故障，不能把环境修好了再继续比较。拒绝和原始提议一起保存。
		state, message = "rejected", "评测策略拒绝全部写操作，已保留提议，未执行"
		if _, err := tx.ExecContext(ctx, "UPDATE approvals SET status='rejected',decided_by='eval-policy',decided_at=UTC_TIMESTAMP(6) WHERE id=? AND status='pending'", id); err != nil {
			return "", "", err
		}
	}
	return state, message, nil
}

func checkDecisionMode(trigger string, approved bool) error {
	if approved && trigger == "eval" {
		return fmt.Errorf("评测诊断不允许批准写操作")
	}
	return nil
}

func checkExecutionMode(trigger string) error {
	if trigger == "eval" {
		return fmt.Errorf("评测诊断不能领取写操作，即使审批记录被误改为批准")
	}
	return nil
}
