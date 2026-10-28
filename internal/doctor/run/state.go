// 本文件：列出本阶段允许的诊断状态变化。
//
// 如果散落在各处直接改字符串，已经取消的任务可能被迟到的结果改成成功。
// 这里先检查变化是否合法，仓储还会用 WHERE status=? 防止并发覆盖。
//
// 主要内容：状态常量用于协议与数据库；Transition 检查所有可达边界。
// 阅读建议：终态没有出边。等待审批属于阶段 7，本阶段无法进入。

package run

import "fmt"

// 状态名称同时用于页面和指标，取值有限，不能使用任意错误文字作为状态。
const (
	Pending        = "pending"         // 已入库，还未占用执行名额。
	Running        = "running"         // 当前进程正在诊断。
	Succeeded      = "succeeded"       // 报告通过结构和证据校验。
	Failed         = "failed"          // 执行或审计失败。
	Cancelled      = "cancelled"       // 用户取消或告警已过时。
	BudgetExceeded = "budget_exceeded" // 最后一次机会仍没有合法报告。
)

// Transition 的表只允许向前推进，不能把终态重新排队。
func Transition(from, to string) error {
	allowed := map[string][]string{Pending: {Running, Cancelled}, Running: {Succeeded, Failed, Cancelled, BudgetExceeded}}
	for _, candidate := range allowed[from] {
		if candidate == to {
			return nil
		}
	}
	return fmt.Errorf("非法诊断状态转换：%s -> %s", from, to)
}
