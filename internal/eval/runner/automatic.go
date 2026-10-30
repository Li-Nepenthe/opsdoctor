// 本文件：按冻结顺序自动完成注入、五组诊断、恢复和下一轮等待。
//
// 长时间评测可能遇到 Docker 退出。每一段的完成状态都在数据库里，续跑先读
// 状态。对执行一半的注入先检查真实症状，不重新执行一遍破坏性的动作。
//
// 主要内容：automatic 遍历冻结轮次；advanceTrial 执行一个可恢复阶段；
// waitBaseline 等待正常流量和告警开放期；matchRecord 防止续接另一场故障。
// 阅读建议：失败时保留恢复文件和暂停记录，不能把失败轮次偷偷换成新的一轮。

package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	chaos "github.com/Li-Nepenthe/opsdoctor/internal/chaos/app"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// automatic 从已冻结的批次续跑；命令本身不能创建另一份随机安排。
func automatic(parent context.Context, options command, reader config.Reader, repository *store.Store, client *doctorClient, output io.Writer) (problem error) {
	if err := checkClock(parent); err != nil {
		return err
	}
	ctx, stop := guardClock(parent, checkClock)
	defer stop()
	// 流量动作可以暂时停流量或换热点，但整个流程都不能换工作集和工作者数。
	loadContext, stopLoad, err := guardLoad(ctx, func(checkContext context.Context) error { return checkWorkload(checkContext, reader) })
	if err != nil {
		return err
	}
	defer stopLoad()
	ctx = loadContext
	status, err := client.runtime(ctx)
	if err != nil {
		return err
	}
	plan, err := pairedPlan(ctx, options, repository, status, false)
	if err != nil {
		return err
	}
	// 宿主机退出之后，评测模式仍留在数据库，自动告警不能趁重启付费。
	_, mode, err := client.fault(ctx, options.target)
	if err != nil {
		return err
	}
	if !mode.Enabled || mode.RunID != options.runID {
		return fmt.Errorf("批次没有持有评测模式，请先 freeze")
	}
	if err = recordEvaluator(ctx, options, repository, "run", ""); err != nil {
		return err
	}
	if err = repository.SetEvalRunState(ctx, options.runID, "running"); err != nil {
		return err
	}
	defer func() {
		if problem != nil {
			if ctx.Err() != nil {
				problem = context.Cause(ctx)
			}
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
			defer cancel()
			_ = repository.SetEvalRunState(cleanup, options.runID, "interrupted")
		}
	}()
	for _, order := range plan.Orders {
		options.scenario, options.iteration = order.Scenario, order.Iteration
		for {
			trial, err := repository.EvalTrial(ctx, options.runID, order.Scenario, order.Iteration)
			if err != nil {
				return err
			}
			if trial.State == "done" {
				break
			}
			fmt.Fprintf(output, "轮次 %s/%d state=%s\n", trial.Scenario, trial.Iteration, trial.State)
			if err = recordEvaluator(ctx, options, repository, trial.State, ""); err != nil {
				return err
			}
			if err = advanceTrial(ctx, options, trial, reader, repository, client, output); err != nil {
				cleanup, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
				if ctx.Err() != nil {
					err = context.Cause(ctx)
				}
				saved := repository.InterruptEval(cleanup, trial, err.Error())
				cancel()
				return errors.Join(err, saved)
			}
		}
	}
	if err = repository.SetEvalRunState(ctx, options.runID, "completed"); err != nil {
		return err
	}
	return finish(ctx, options, repository, client, output)
}

// advanceTrial 每次只前进一段。失败不改状态，下次会重新核对这一段的事实。
func advanceTrial(ctx context.Context, options command, trial store.EvalTrial, reader config.Reader, repository *store.Store, client *doctorClient, output io.Writer) error {
	engine := chaos.Engine{StatePath: ".opsdoctor-chaos.json"}
	switch trial.State {
	case "pending":
		if _, err := engine.Read(); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("仍有活动恢复记录或记录无法读取，请先核对并恢复原实验")
		}
		if err := waitBaseline(ctx, options, reader, client, output); err != nil {
			return err
		}
		if err := repository.AdvanceEvalTrial(ctx, trial, "injecting", nil); err != nil {
			return err
		}
		// 先写 injecting。命令响应丢失时，下一轮先查看记录和症状，不再注入。
		if err := chaos.Run(ctx, []string{"inject", trial.Scenario}, reader, output); err != nil {
			return err
		}
		record, err := matchRecord(engine, trial)
		if err != nil {
			return err
		}
		trial.State = "injecting"
		return repository.AdvanceEvalTrial(ctx, trial, "verifying", &record.Started)
	case "injecting":
		record, err := matchRecord(engine, trial)
		if err != nil {
			return fmt.Errorf("注入结果不确定，不能自动重做；请核对本轮恢复记录：%w", err)
		}
		// 已存在记录只证明开始过；仍需 verify 确认所有预期症状。
		return repository.AdvanceEvalTrial(ctx, trial, "verifying", &record.Started)
	case "verifying":
		if _, err := matchRecord(engine, trial); err != nil {
			return err
		}
		if err := chaos.Run(ctx, []string{"verify", trial.Scenario}, reader, output); err != nil {
			return fmt.Errorf("症状未在原三分钟预算内确认，保留失败轮次；请先核对环境，不重新注入：%w", err)
		}
		return repository.AdvanceEvalTrial(ctx, trial, "diagnosing", nil)
	case "diagnosing":
		if _, err := matchRecord(engine, trial); err != nil {
			return err
		}
		if err := matrix(ctx, options, reader, repository, client, output); err != nil {
			return err
		}
		return repository.AdvanceEvalTrial(ctx, trial, "restoring", nil)
	case "restoring":
		_, err := matchRecord(engine, trial)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil {
			if err = chaos.Run(ctx, []string{"restore", trial.Scenario}, reader, output); err != nil {
				return err
			}
		}
		// 恢复文件只在所有恢复动作完成后删除。若在删除后崩溃，多等五分钟即可。
		return repository.AdvanceEvalTrial(ctx, trial, "cooling", nil)
	case "cooling":
		if err := waitBaseline(ctx, options, reader, client, output); err != nil {
			return err
		}
		for trial.CooldownSeconds > 0 {
			if err := waitTick(ctx, 10*time.Second); err != nil {
				return err
			}
			var err error
			trial, err = repository.EvalTrial(ctx, trial.RunID, trial.Scenario, trial.Iteration)
			if err != nil {
				return err
			}
		}
		// 冷却中也可能出现延迟到达的告警，所以最后再读一次开放期。
		if err := waitBaseline(ctx, options, reader, client, output); err != nil {
			return err
		}
		return repository.AdvanceEvalTrial(ctx, trial, "done", nil)
	default:
		return fmt.Errorf("未知实验状态 %s，停止续跑", trial.State)
	}
}

// matchRecord 必须匹配同一次注入；同名场景在另一天重注入也不能接上旧配对。
func matchRecord(engine chaos.Engine, trial store.EvalTrial) (chaos.Record, error) {
	record, err := engine.Read()
	if err != nil {
		return record, err
	}
	if record.Scenario.ID != trial.Scenario || (trial.FaultStartedAt != nil && !trial.FaultStartedAt.Equal(record.Started.UTC().Truncate(time.Microsecond))) {
		return record, fmt.Errorf("恢复记录与评测轮次不符")
	}
	return record, nil
}

// waitBaseline 等待的是 API 的真实状态，不用固定睡眠猜测三分钟观察期结束。
func waitBaseline(parent context.Context, options command, reader config.Reader, client *doctorClient, output io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, 20*time.Minute)
	defer cancel()
	if err := checkBaseline(ctx, reader); err != nil {
		return err
	}
	if _, err := memoryBytes(ctx); err != nil {
		return err
	}
	if err := chaos.Run(ctx, []string{"healthy"}, reader, output); err != nil {
		return err
	}
	// healthy 通过后仍可能处在三分钟恢复观察期；继续读服务端的开放状态。
	for {
		fault, _, err := client.fault(ctx, options.target)
		if err != nil {
			return err
		}
		if !fault.Open {
			return nil
		}
		if err = waitTick(ctx, 5*time.Second); err != nil {
			return err
		}
	}
}

// waitTick 只暂停当前流程，取消时立刻返回，不留下定时器或后台付费调用。
func waitTick(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-timer.C:
		return nil
	}
}
