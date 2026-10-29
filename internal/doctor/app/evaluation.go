// 本文件：把实验组的含义落实为真正不同的工具菜单。
//
// 模型没有“切换到另一组”的参数。调度器从已经保存的任务读出组别，再创建独立菜单。
// 这里不读取场景文件，也不知道正确根因；只知道 E0–E3 各能使用哪些信息。
//
// 主要内容：prepareDiagnosis 设置故障时间，移除 E0 的全部工具和 E3 的调用链工具。
// 阅读建议：移除工具要同时移除执行入口，不能只在提示词里说“不许用”。

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
