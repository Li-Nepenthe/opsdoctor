// 本文件：按任务绑定的执行配置一起选择模型、仓储和预算账户，校验配置与费用边界，并提供各账户的独立运行状态。

package run

import (
	"context"
	"errors"
	"fmt"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
)

// Execution 的内容在启动时固定，各任务只持有引用，不会修改其他正在执行的任务。
type Execution struct {
	Model      string                                      // 与任务保存的预期模型一致才允许执行。
	Provider   string                                      // mock 不收费；真实调用仍经过原有预留与结算。
	Account    string                                      // 固定预算账户，不接收模型生成的名字。
	Agent      *agent.Agent                                // 模型客户端、提示和限制都在构造时绑定。
	Repository Repository                                  // 同一 doctor 数据库，但使用这套配置的币种。
	Budget     func(context.Context) (BudgetStatus, error) // 只查询本配置账户。
}

// ExecutionStatus 只含可公开的运行信息，不包含 Key、地址、提示或客户端对象。
type ExecutionStatus struct {
	Model    string       `json:"model"` // 实际装配的模型，不能只信宿主机 .env。
	Provider string       `json:"provider"`
	Account  string       `json:"account"` // 已授权预算账户的名字。
	Budget   BudgetStatus `json:"budget"`  // 该账户币种、用量和停用原因。
}

// ExecutionStatuses 按账户分别查询，不能把两种货币加在同一个余额里。
func (scheduler *Scheduler) ExecutionStatuses(ctx context.Context) (map[string]ExecutionStatus, error) {
	values := map[string]ExecutionStatus{}
	for name, execution := range scheduler.config.Executions {
		budget, err := execution.Budget(ctx)
		if err != nil {
			return nil, err // 查不到就明确失败，不能显示为零花费。
		}
		scheduler.mu.Lock()
		blocked := scheduler.accountBlocked[execution.Account]
		if execution.Account == scheduler.config.BudgetAccount {
			blocked = blocked || scheduler.blocked
		}
		scheduler.mu.Unlock()
		if blocked {
			budget.Blocked = true
			if budget.Reason == "" {
				budget.Reason, budget.ReasonCode = "账户不足以预留下一次请求", "request_reservation"
			}
		}
		values[name] = ExecutionStatus{Model: execution.Model, Provider: execution.Provider, Account: execution.Account, Budget: budget}
	}
	return values, nil
}

// executionFor 拒绝没有注册的配置，不能为了继续实验而悄悄换回默认模型。
func (scheduler *Scheduler) executionFor(task Task) (Execution, error) {
	value := Execution{Model: scheduler.config.Model, Provider: scheduler.config.Provider, Account: scheduler.config.BudgetAccount, Agent: scheduler.agent, Repository: scheduler.repository, Budget: scheduler.config.Budget}
	if task.ExecutionProfile != "" {
		var ok bool
		value, ok = scheduler.config.Executions[task.ExecutionProfile]
		if !ok || task.EvaluationGroup == "" {
			return Execution{}, fmt.Errorf("评测执行配置未启用，不能回退到其他模型")
		}
	}
	if task.ExpectedModel != "" && task.ExpectedModel != value.Model {
		return Execution{}, fmt.Errorf("评测任务的模型与执行配置不一致")
	}
	return value, nil
}

// configuredExecutions 在启动时复制执行配置。默认使用 Mock 时，评测也不能切到真实模型。
func configuredExecutions(config Config) (map[string]Execution, error) {
	executions := make(map[string]Execution, len(config.Executions))
	for name, execution := range config.Executions {
		if name == "" || execution.Model == "" || execution.Agent == nil || execution.Repository == nil || execution.Budget == nil || (execution.Provider != "mock" && execution.Provider != "openai") || (config.Provider == "mock" && execution.Provider != "mock") {
			return nil, fmt.Errorf("评测执行配置不完整")
		}
		executions[name] = execution
	}
	return executions, nil
}

// canDispatch 每次分派都检查本配置绑定的账本；Mock 也会读取余额，但不会因余额不足而停用。
func (scheduler *Scheduler) canDispatch(lookup context.Context, execution Execution) (bool, error) {
	status, err := execution.Budget(lookup)
	if err != nil {
		return false, err
	}
	scheduler.mu.Lock()
	blocked := scheduler.accountBlocked[execution.Account]
	if execution.Account == scheduler.config.BudgetAccount {
		blocked = scheduler.blocked
	}
	scheduler.mu.Unlock()
	return execution.Provider == "mock" || (!status.Blocked && !blocked), nil
}

// budgetFailure 只在账户预留不足时停用这个账户。单次评测用尽预算不会影响下一次诊断。
func (scheduler *Scheduler) budgetFailure(account string, err error) (string, bool) {
	if errors.Is(err, llm.ErrBudget) {
		scheduler.mu.Lock()
		if account == scheduler.config.BudgetAccount {
			scheduler.blocked = true
		} else {
			scheduler.accountBlocked[account] = true
		}
		scheduler.mu.Unlock()
		return llm.ErrBudget.Error(), true
	}
	if errors.Is(err, llm.ErrEvaluationBudget) {
		return err.Error(), true
	}
	return "", false
}
