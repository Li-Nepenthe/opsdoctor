// 本文件：记录外部靶场首次确认症状和恢复健康所需的实际时间。
//
// UTC 时间方便与其他证据对照，耗时则只使用同一进程保留下来的单调时钟。
// 续跑无法找回原来的单调时钟，必须明确写成未知，不能用数据库时间近似替代。
//
// 主要内容：withDemoTiming 保存本轮锚点；rememberDemoInjection、rememberDemoRestore
// 记录开始；observeDemoSymptom、observeDemoRecovery 独占写首次通过的证据。
// 阅读建议：这里不查询指标，也不改变谓词、轮询或期限；恢复计时不含后续冷却。

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// demoTimingKey 只在宿主评测器上下文中传递，不进入医生输入或数据库配置。
type demoTimingKey struct{}

// demoTimingState 的时间保留单调部分；root 供测试把全部证据隔离到临时目录。
type demoTimingState struct {
	mu      sync.Mutex           // 同一锚点只允许记一次，不让后来的调用刷新耗时。
	anchors map[string]time.Time // 场景轮次和观察类型一起标识一次计时。
	root    string               // 生产固定在系统临时目录的 recovery 下。
}

// demoTimingObservation 分开公开 UTC 与单调耗时；未知时没有任何猜测的耗时数字。
type demoTimingObservation struct {
	RunID          string     `json:"run_id"`                    // 不与其他批次合并。
	Scenario       string     `json:"scenario"`                  // 外部场景编号。
	Iteration      int        `json:"iteration"`                 // 对应本轮恢复文件。
	Kind           string     `json:"kind"`                      // symptom 或 recovery。
	StartUTC       *time.Time `json:"start_utc,omitempty"`       // 只作时间对照，不用来猜耗时。
	PassedUTC      time.Time  `json:"passed_utc"`                // 首次谓词通过后的实际观察时刻。
	Known          bool       `json:"monotonic_elapsed_known"`   // false 表示原单调锚点不可用。
	ElapsedNS      *int64     `json:"elapsed_ns,omitempty"`      // 只有同进程单调时钟可用时填写。
	ElapsedSeconds *float64   `json:"elapsed_seconds,omitempty"` // 同一耗时的秒数表示。
	Polls          int        `json:"polls"`                     // 本次等待实际执行的查询次数。
}

// withDemoTiming 每次运行新建状态，不能从旧恢复文件伪造单调锚点。
func withDemoTiming(ctx context.Context) context.Context {
	state := &demoTimingState{anchors: map[string]time.Time{}, root: filepath.Join(os.TempDir(), "opsdoctor-phase09", "recovery")}
	return context.WithValue(ctx, demoTimingKey{}, state)
}

// rememberDemoInjection 在写入 Started 的 UTC 副本前保留原始 time.Now()。
func rememberDemoInjection(ctx context.Context, options generalizationCommand, at time.Time) error {
	return rememberDemoTime(ctx, options, "symptom", at)
}

// rememberDemoRestore 只记录本次写开关且读回一致的时刻，不能从旧确认文件刷新。
func rememberDemoRestore(ctx context.Context, options generalizationCommand, at time.Time) error {
	return rememberDemoTime(ctx, options, "recovery", at)
}

// rememberDemoTime 拒绝覆盖原锚点，避免重复调用把已经经过的等待时间抹掉。
func rememberDemoTime(ctx context.Context, options generalizationCommand, kind string, at time.Time) error {
	state, path, err := demoTimingDestination(ctx, options, kind)
	if err != nil {
		return err
	}
	if at.IsZero() {
		return fmt.Errorf("阶段9 Demo 计时起点为空")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	// 重复传入同一时刻可以确认原锚点，但不能用后来的一次调用重新开始计时。
	if prior, ok := state.anchors[path]; ok {
		if !prior.Equal(at) {
			return fmt.Errorf("阶段9 Demo 原计时起点已存在，不能刷新")
		}
		return nil
	}
	state.anchors[path] = at
	return nil
}

// observeDemoSymptom 在首次症状谓词通过时保存从注入 Started 开始的耗时。
func observeDemoSymptom(ctx context.Context, options generalizationCommand, passedAt time.Time, polls int) error {
	return observeDemoTime(ctx, options, "symptom", passedAt, polls)
}

// observeDemoRecovery 在首次健康谓词通过时保存从恢复读回开始的耗时，不含冷却。
func observeDemoRecovery(ctx context.Context, options generalizationCommand, passedAt time.Time, polls int) error {
	return observeDemoTime(ctx, options, "recovery", passedAt, polls)
}

// observeDemoTime 不补造续跑丢失的单调时钟，也不覆盖已经保存的首次观察。
func observeDemoTime(ctx context.Context, options generalizationCommand, kind string, passedAt time.Time, polls int) error {
	state, path, err := demoTimingDestination(ctx, options, kind)
	if err != nil {
		return err
	}
	if passedAt.IsZero() || polls < 1 {
		return fmt.Errorf("阶段9 Demo 观察时间或查询次数无效")
	}
	state.mu.Lock()
	start, exists := state.anchors[path]
	state.mu.Unlock()
	value := demoTimingObservation{RunID: options.runID, Scenario: options.scenario, Iteration: options.iteration, Kind: kind, PassedUTC: passedAt.UTC(), Polls: polls}
	if exists {
		utc := start.UTC()
		value.StartUTC = &utc
		// Round(0) 只移除单调部分；UTC()/JSON 解码后的时间不能通过这个检查。
		value.Known = start != start.Round(0) && passedAt != passedAt.Round(0)
		if value.Known {
			elapsed := passedAt.Sub(start)
			if elapsed < 0 {
				return fmt.Errorf("阶段9 Demo 首次通过早于原计时起点")
			}
			nanoseconds, seconds := elapsed.Nanoseconds(), elapsed.Seconds()
			value.ElapsedNS, value.ElapsedSeconds = &nanoseconds, &seconds
		}
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = writeCollectorEvidence(path, raw); err != nil {
		return fmt.Errorf("阶段9 Demo 首次观察证据未能独占保存：%w", err)
	}
	return nil
}

// demoTimingDestination 复用恢复文件的身份校验，只允许在所属轮次旁增加证据文件。
func demoTimingDestination(ctx context.Context, options generalizationCommand, kind string) (*demoTimingState, string, error) {
	state, ok := ctx.Value(demoTimingKey{}).(*demoTimingState)
	if !ok || state == nil || state.anchors == nil || state.root == "" {
		return nil, "", fmt.Errorf("阶段9 Demo 本次运行未安装计时状态")
	}
	if kind != "symptom" && kind != "recovery" {
		return nil, "", fmt.Errorf("阶段9 Demo 观察类型无效")
	}
	path, err := demoRecoveryPath(options)
	if err != nil {
		return nil, "", err
	}
	name := strings.TrimSuffix(filepath.Base(path), ".json") + "." + kind + "-observed.json"
	return state, filepath.Join(state.root, options.runID, name), nil
}
