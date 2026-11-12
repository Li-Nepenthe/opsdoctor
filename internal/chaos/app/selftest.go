// 本文件：Trial 独立核对每次实验的初始健康、观察和恢复结果。
// Selftest 遇到恢复失败时停止后续场景，在结果表中保留失败和未执行的行。

package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
)

// 注入和恢复各自有时限，实际动作耗时计入相应阶段。
const phaseBudget = 3 * time.Minute

// Row 中的时间都来自实际时钟，尚未执行的步骤保持零值并在 Problem 中说明。
type Row struct {
	ID       string // 场景编号，仅出现在宿主机结果表。
	Before   time.Duration
	Inject   time.Duration
	Verified time.Duration // 从注入开始到症状和告警均被看到。
	Restore  time.Duration // 从恢复开始到连续一分钟健康。
	Alerts   []string
	Problem  error // 任一步失败都不能被后面的恢复成功覆盖。
	Restored bool  // false 时不得继续下一场景。
}

// Trial 先确认健康，再开始计时；原本已经有故障的系统不能用来判断本次注入的效果。
func (engine *Engine) Trial(ctx context.Context, value scenario.Scenario) (row Row) {
	row.ID = value.ID
	budget, err := time.ParseDuration(value.Symptom.Within)
	if err != nil || budget <= 0 || budget > phaseBudget {
		row.Problem = fmt.Errorf("场景观察时限不合法")
		return row
	}
	healthCtx, cancel := context.WithTimeout(ctx, phaseBudget)
	health, err := engine.Observer.Healthy(healthCtx)
	cancel()
	row.Before = health.Elapsed
	if err != nil {
		row.Problem = err
		return row
	}
	start := time.Now()
	// 场景可以要求更短的观察时限，但不能超过阶段规定的三分钟。
	injectCtx, cancel := context.WithTimeout(ctx, budget)
	record, injectErr := engine.Inject(injectCtx, value)
	row.Inject = time.Since(start)
	if injectErr == nil {
		observed, verifyErr := engine.Observer.Symptom(injectCtx, value.Symptom)
		row.Alerts = observed.Alerts
		row.Verified = time.Since(start)
		row.Problem = verifyErr
	} else {
		row.Problem = injectErr
	}
	cancel()
	// 实验取消后仍要恢复，清理使用独立的有限预算。
	// 只有本次确实创建了记录才能恢复；不能触碰开工前就存在的另一个实验。
	saved, readErr := engine.Read()
	if readErr != nil || !saved.Started.Equal(record.Started) {
		row.Problem = errors.Join(row.Problem, readErr)
		return row
	}
	restoreStart := time.Now()
	restoreCtx, restoreCancel := context.WithTimeout(context.WithoutCancel(ctx), phaseBudget)
	defer restoreCancel()
	err = engine.Restore(restoreCtx, record)
	if err == nil {
		_, err = engine.Observer.Healthy(restoreCtx)
	}
	row.Restore = time.Since(restoreStart)
	row.Restored = err == nil
	row.Problem = errors.Join(row.Problem, err)
	return row
}

// Selftest 不在场景间自动补库存，操作者须在实验前显式准备，避免把数据变化藏进流程。
func (engine *Engine) Selftest(ctx context.Context, values []scenario.Scenario, output io.Writer) error {
	// 先检查遗留实验，不能把它恢复前的状态采成新基线。
	if _, err := os.Stat(engine.StatePath); err == nil {
		return fmt.Errorf("仍有未恢复实验，请先执行 restore-all")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("无法检查恢复记录：%w", err)
	}
	fmt.Fprintln(output, "| 场景 | 事前健康 | 注入耗时 | 症状及告警（自注入起） | 恢复健康（含恢复动作） | 告警 | 结果 |")
	fmt.Fprintln(output, "|---|---:|---:|---:|---:|---|---|")
	var problems []error
	for index, value := range values {
		row := engine.Trial(ctx, value)
		outcome := "通过"
		if row.Problem != nil {
			outcome = strings.NewReplacer("\n", "；", "|", "/").Replace(row.Problem.Error())
			problems = append(problems, fmt.Errorf("%s：%w", row.ID, row.Problem))
		}
		fmt.Fprintf(output, "| %s | %s | %s | %s | %s | %s | %s |\n", row.ID, row.Before.Round(time.Millisecond), row.Inject.Round(time.Millisecond), row.Verified.Round(time.Millisecond), row.Restore.Round(time.Millisecond), strings.Join(row.Alerts, ","), outcome)
		if !row.Restored || ctx.Err() != nil {
			// 恢复不完整时继续注入会混合原因，后面的结果也不再可信。
			// 保留剩余编号，但明确标成未执行，避免读者把缺行误解为全部通过。
			for _, pending := range values[index+1:] {
				fmt.Fprintf(output, "| %s | — | — | — | — | — | 未执行：前序基线或恢复未通过，或实验被取消 |\n", pending.ID)
			}
			break
		}
	}
	return errors.Join(problems...)
}
