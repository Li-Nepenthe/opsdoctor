// 本文件：调度器从已保存的任务读取实验组，交给 prepareDiagnosis 限定证据时间和工具。
// 运行时的菜单与实际执行入口使用同一范围，模型不能自行切换组别。

package app

import (
	"fmt"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// prepareDiagnosis 对普通诊断也设置时间边界；评测组只改变可用数据，不注入场景提示。
func prepareDiagnosis(task run.Task, input *agent.Input) error {
	registry, ok := input.Tools.(*tools.Registry)
	if !ok {
		return fmt.Errorf("诊断工具没有使用统一注册表")
	}
	allowed := map[string]bool{}
	for _, item := range registry.List() {
		allowed[item.Name] = true
	}
	switch task.EvaluationGroup {
	case "", "E1":
		delete(allowed, "runbook_search") // 手册只能供 E2 使用。
	case "E0":
		allowed = map[string]bool{}
		input.InputOnly, input.Approvals = true, nil
	case "E2":
		if !allowed["runbook_search"] {
			return fmt.Errorf("E2 的排障手册尚未配置")
		}
	case "E3":
		delete(allowed, "search_traces")
		delete(allowed, "get_trace")
		delete(allowed, "runbook_search")
		input.Unavailable = append(append([]string{}, input.Unavailable...), "调用链（本次工具配置未提供）")
	default:
		return fmt.Errorf("未知实验组，不能回退到默认工具菜单")
	}
	input.FaultStartedAt = task.FaultStartedAt
	input.Tools = registry.ForDiagnosis(task.FaultStartedAt, allowed)
	return nil
}
