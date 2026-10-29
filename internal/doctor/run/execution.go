// 本文件：为一份评测任务选择已经配置好的模型和账本。
//
// 比较两个模型不需要重启医生。每份任务保存一个配置名，开始时只从启动时的
// 白名单里选择。模型、仓储币种和预算一起选择，不能只换模型却沿用另一家的账本。
//
// 主要内容：Execution 保存一套执行依赖；executionFor 校验任务的模型绑定。
// ExecutionStatuses 返回各配置的模型与余额，供评测器核对运行中的真实配置。
// 阅读建议：这里没有 URL、Key 或场景答案，模型也不能请求切换配置。

package run

import (
	"context"
	"fmt"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
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
	Model    string       `json:"model"`    // 实际装配的模型，不能只信宿主机 .env。
	Provider string       `json:"provider"` // mock 或真实协议客户端。
	Account  string       `json:"account"`  // 已授权预算账户的名字。
	Budget   BudgetStatus `json:"budget"`   // 该账户币种、用量和停用原因。
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
