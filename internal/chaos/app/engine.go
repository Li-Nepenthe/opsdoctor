// 本文件：组织一次实验，并把尚未恢复的状态留在宿主机。
//
// 外部动作可能执行成功却没有返回确认。先落盘恢复记录，再开始动作，进程退出后仍有依据恢复。
// selftest 的结果按行输出，失败也保留，不能只留下成功的轮次。
//
// 主要内容：
//   - Executor、Observer：只声明流程需要的能力，测试用假的实现替换。
//   - Engine、Record：连接场景、动作与本机恢复记录。
//   - Inject、Restore：保证修改前有记录，恢复全部成功后才删除记录。
//
// 阅读建议：先看 Inject 的写入顺序，再看 Restore 如何继续执行剩余的清理动作。

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/action"
	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/verify"
)

// Executor 隔离真正的 Docker、数据库和网络操作，流程测试无需启动容器。
type Executor interface {
	Capture(context.Context, scenario.Scenario) (action.Baseline, error) // 必须在第一项改动之前完成。
	Execute(context.Context, scenario.Step) error                        // 执行一项已经校验的动作。
	Restore(context.Context, scenario.Step, action.Baseline) error       // 根据原配置恢复。
}

// Observer 只看监控结果，不借用注入器内部状态判断故障是否出现。
type Observer interface {
	Healthy(context.Context) (verify.Result, error)                   // 连续一分钟正常才通过。
	Symptom(context.Context, scenario.Symptom) (verify.Result, error) // 同时确认症状与相关告警。
}

// Record 保留当时的完整场景，恢复时不依赖后来可能已编辑过的 YAML。
type Record struct {
	Scenario scenario.Scenario `json:"scenario"` // 本机文件包含答案，不能挂进业务或医生容器。
	Baseline action.Baseline   `json:"baseline"` // 公开配置和镜像内容 ID，没有密码。
	Started  time.Time         `json:"started"`  // 三分钟计时从第一项注入开始。
}

// Engine 用一个独占状态文件防止两个场景叠加，否则难以知道该恢复哪份基线。
type Engine struct {
	Executor  Executor // 外部动作由这个对象执行。
	Observer  Observer // 健康与症状由监控后端提供。
	StatePath string   // 默认仓库根的忽略文件，不写入 Git。
}

// Inject 用 O_EXCL 独占创建记录。已有记录时停止，不能覆盖另一次未恢复的实验。
func (engine *Engine) Inject(ctx context.Context, value scenario.Scenario) (Record, error) {
	// 先取得可恢复的基线。捕获失败时还没有副作用，可以直接退出。
	baseline, err := engine.Executor.Capture(ctx, value)
	if err != nil {
		return Record{}, err
	}
	record := Record{Scenario: value, Baseline: baseline, Started: time.Now()}
	file, err := os.OpenFile(engine.StatePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return record, fmt.Errorf("不能创建恢复记录，请先检查或执行 restore-all：%w", err)
	}
	writeErr := json.NewEncoder(file).Encode(record)
	// Sync 要求操作系统刷新文件，再开始改外部环境，缩小掉电丢记录的窗口。
	syncErr := file.Sync()
	closeErr := file.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return record, err
	}
	for _, step := range value.Inject {
		if err = engine.Executor.Execute(ctx, step); err != nil {
			return record, fmt.Errorf("注入动作 %s 未完成：%w", step.Action, err)
		}
	}
	return record, nil
}

// Read 在恢复或单独 verify 时使用原记录，避免错把另一场景的状态拿来验收。
func (engine *Engine) Read() (Record, error) {
	var record Record
	data, err := os.ReadFile(engine.StatePath)
	if err != nil {
		return record, err
	}
	if err = json.Unmarshal(data, &record); err != nil {
		return record, fmt.Errorf("恢复记录无法解析，保留文件供核对：%w", err)
	}
	return record, nil
}

// Restore 尝试全部恢复动作；一个动作失败不能阻止另一条网络链路恢复。
func (engine *Engine) Restore(ctx context.Context, record Record) error {
	var problems []error
	for _, step := range record.Scenario.Restore {
		if err := engine.Executor.Restore(ctx, step, record.Baseline); err != nil {
			problems = append(problems, fmt.Errorf("恢复动作 %s：%w", step.Action, err))
		}
	}
	if err := errors.Join(problems...); err != nil {
		return err
	}
	// 只有所有动作明确成功才移除记录；否则下次 restore-all 可以继续重试。
	return os.Remove(engine.StatePath)
}
