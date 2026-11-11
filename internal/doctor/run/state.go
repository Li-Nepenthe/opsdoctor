// 本文件：限制诊断状态可以怎样推进。仓储写入时再核对旧状态，避免迟到的执行结果覆盖已经取消的任务。

package run

import "fmt"

// 状态名称同时用于页面和指标，取值有限，不能使用任意错误文字作为状态。
const (
	Pending         = "pending"          // 已入库，还未占用执行名额。
	WaitingApproval = "waiting_approval" // 计划已经保存，等待人决定。
	Running         = "running"          // 当前进程正在诊断。
	Succeeded       = "succeeded"        // 报告通过结构和证据校验。
	Failed          = "failed"           // 执行或审计失败。
	Cancelled       = "cancelled"        // 用户取消或告警已过时。
	BudgetExceeded  = "budget_exceeded"  // 最后一次机会仍没有合法报告。
)

// Transition 的表只允许向前推进，不能把终态重新排队。
func Transition(from, to string) error {
	allowed := map[string][]string{Pending: {Running, Cancelled}, Running: {WaitingApproval, Succeeded, Failed, Cancelled, BudgetExceeded}, WaitingApproval: {Running, Failed, Cancelled}}
	for _, candidate := range allowed[from] {
		if candidate == to {
			return nil
		}
	}
	return fmt.Errorf("非法诊断状态转换：%s -> %s", from, to)
}
