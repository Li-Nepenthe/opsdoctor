// 本文件：给阶段9的健康基线留出观察窗口，并独占保存每次调用的真实证据。
//
// 保持资源、开关、健康、故障开放期的原检查顺序。只有健康条件允许等待，
// 同一轮全部场景通过才继续；配置不符或查询错误都不靠下一轮掩盖。
//
// 主要内容：demoBaseline装配真实依赖；runDemoBaseline记录整次调用；
// waitDemoBaselineHealth执行五秒轮询，不改变场景谓词或医生能力。

package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// demoBaselineInterval固定生产轮询间隔，测试替换等待动作但不改变传入的间隔。
const demoBaselineInterval = 5 * time.Second

// demoBaselineSources让单元测试模拟所有外部依赖，不访问Docker或医生接口。
type demoBaselineSources struct {
	memory func(context.Context) error
	flags  func(context.Context) (json.RawMessage, error)
	fault  func(context.Context) (bool, error)
	query  func(context.Context, string) (bool, error)
	wait   func(context.Context, time.Duration) error
	root   string // 测试使用临时目录；生产缺省与其他阶段9证据相邻。
}

// demoBaselineObservation记录一次调用，失败也保留耗时与实际轮数，不拼接别次健康样本。
type demoBaselineObservation struct {
	RunID                string     `json:"run_id"`
	Scenario             string     `json:"scenario"` // freeze尚无轮次，明确保存空字符串和0。
	Iteration            int        `json:"iteration"`
	Kind                 string     `json:"kind"`
	Phase                string     `json:"phase"` // freeze、pre-inject或post-cool，不能互相冒充。
	Cases                []string   `json:"scenarios"`
	StartUTC             time.Time  `json:"start_utc"`
	FinishedUTC          time.Time  `json:"finished_utc"`
	PassedUTC            *time.Time `json:"passed_utc,omitempty"`
	Known                bool       `json:"monotonic_elapsed_known"`
	ElapsedNS            int64      `json:"elapsed_ns"`
	ElapsedSeconds       float64    `json:"elapsed_seconds"`
	HealthStartUTC       *time.Time `json:"health_start_utc,omitempty"`
	HealthFinishedUTC    *time.Time `json:"health_finished_utc,omitempty"`
	HealthKnown          bool       `json:"health_monotonic_elapsed_known"`
	HealthElapsedNS      *int64     `json:"health_elapsed_ns,omitempty"` // 仅健康轮询，与五分钟上限对应。
	HealthElapsedSeconds *float64   `json:"health_elapsed_seconds,omitempty"`
	Polls                int        `json:"polls"` // 一轮包含全部选中场景，不是单条查询数量。
	Queries              int        `json:"queries"`
	Status               string     `json:"status"` // passed或failed，配置/查询/开放期失败也留原件。
	Error                string     `json:"error,omitempty"`
}

// demoBaseline保持原调用接口，freeze和每轮注入前分别标记来源。
func demoBaseline(ctx context.Context, options generalizationCommand, cases map[string]demoScenario, client *doctorClient) error {
	phase := "pre-inject"
	if options.action == "freeze" {
		phase = "freeze"
	}
	return demoBaselinePhase(ctx, options, cases, client, phase)
}

// demoBaselinePhase只增加观察与证据，不修改原资源、开关和故障开放期的判断。
func demoBaselinePhase(ctx context.Context, options generalizationCommand, cases map[string]demoScenario, client *doctorClient, phase string) error {
	sources := demoBaselineSources{
		memory: func(ctx context.Context) error { _, err := demoMemoryBytes(ctx); return err },
		flags:  func(ctx context.Context) (json.RawMessage, error) { return readDemoFlags(ctx, options.flagd) },
		fault: func(ctx context.Context) (bool, error) {
			value, _, err := client.fault(ctx, options.target)
			return value.Open, err
		},
		query: func(ctx context.Context, query string) (bool, error) {
			return demoQuery(ctx, options.prometheus, query)
		},
		wait: waitTick,
	}
	// run沿用其他观察原件目录，freeze没有计时状态时使用同一个生产缺省目录。
	if state, ok := ctx.Value(demoTimingKey{}).(*demoTimingState); ok && state != nil {
		sources.root = state.root
	}
	return runDemoBaseline(ctx, options, cases, phase, sources)
}

// runDemoBaseline使用本调用原始time.Now单调时钟；任何落盘错误都阻止下一步。
func runDemoBaseline(ctx context.Context, options generalizationCommand, cases map[string]demoScenario, phase string, sources demoBaselineSources) (problem error) {
	started := time.Now()
	if !regexp.MustCompile(`^phase09-[A-Za-z0-9_-]{1,56}$`).MatchString(options.runID) || phase != "freeze" && phase != "pre-inject" && phase != "post-cool" {
		return fmt.Errorf("阶段9基线证据身份不合法")
	}
	root := sources.root
	if root == "" {
		root = filepath.Join(os.TempDir(), "opsdoctor-phase09", "recovery")
	}
	name := fmt.Sprintf("baseline-%s-%s.json", phase, started.UTC().Format("20060102T150405.000000000Z"))
	path := filepath.Join(root, options.runID, name)
	value := demoBaselineObservation{RunID: options.runID, Scenario: options.scenario, Iteration: options.iteration, Kind: "baseline", Phase: phase, Cases: append([]string{}, options.cases...), StartUTC: started.UTC(), Known: true, Status: "failed"}
	if phase == "freeze" {
		value.Scenario, value.Iteration = "", 0 // CLI的iteration缺省1不表示freeze已经属于某轮。
	}
	// 失败与成功都保留首次原件；文件名含每次调用时间，freeze和同轮前后不会互相覆盖。
	defer func() {
		finished := time.Now()
		elapsed := finished.Sub(started)
		value.FinishedUTC, value.ElapsedNS, value.ElapsedSeconds = finished.UTC(), elapsed.Nanoseconds(), elapsed.Seconds()
		if problem == nil {
			value.Status = "passed"
			at := finished.UTC()
			value.PassedUTC = &at
		} else {
			value.Error = problem.Error()
		}
		raw, err := json.MarshalIndent(value, "", "  ")
		if err == nil {
			err = writeCollectorEvidence(path, raw)
		}
		if err != nil {
			problem = errors.Join(problem, fmt.Errorf("阶段9基线证据未能独占保存：%w", err))
		}
	}()
	if err := sources.memory(ctx); err != nil {
		return err
	}
	if len(options.baseline) > 0 {
		actual, err := sources.flags(ctx)
		if err != nil || !equalJSON(actual, options.baseline) {
			return fmt.Errorf("外部靶场 Demo 开关与冻结健康配置不同，不能开始新注入：%v", err)
		}
	}
	if err := waitDemoBaselineHealth(ctx, options, cases, sources, &value); err != nil {
		return err
	}
	// 保留原顺序：只有健康通过以后才检查一次医生的故障开放期。
	open, err := sources.fault(ctx)
	if err != nil {
		return err
	}
	if open {
		return fmt.Errorf("外部靶场 Demo 上一场故障仍在开放期，不能开始下一次注入")
	}
	return nil
}

// waitDemoBaselineHealth每轮重新判断全部场景，不能把不同轮的成功拼成同时健康。
func waitDemoBaselineHealth(parent context.Context, options generalizationCommand, cases map[string]demoScenario, sources demoBaselineSources, observation *demoBaselineObservation) error {
	started := time.Now()
	startUTC := started.UTC()
	observation.HealthStartUTC = &startUTC
	// 整次baseline还包括资源、开关和开放期；健康窗口单独留单调耗时，不能混算。
	defer func() {
		finished := time.Now()
		finishUTC := finished.UTC()
		elapsed := finished.Sub(started)
		ns, seconds := elapsed.Nanoseconds(), elapsed.Seconds()
		observation.HealthFinishedUTC, observation.HealthElapsedNS, observation.HealthElapsedSeconds = &finishUTC, &ns, &seconds
		observation.HealthKnown = true
	}()
	ctx, cancel := context.WithTimeout(parent, demoObservationLimit)
	defer cancel()
	for {
		if err := demoBaselineDeadline(ctx); err != nil {
			return fmt.Errorf("阶段9健康基线未在观察窗口内全部通过：%w", err)
		}
		observation.Polls++
		all := len(options.cases) > 0
		for _, name := range options.cases {
			value, ok := cases[name]
			if !ok || value.HealthyPromQL == "" {
				return fmt.Errorf("阶段9场景%s缺少冻结健康查询", name)
			}
			observation.Queries++
			passed, err := sources.query(ctx, value.HealthyPromQL)
			if err != nil {
				return fmt.Errorf("外部靶场 Demo 场景 %s 的健康基线查询失败：%w", name, err)
			}
			if err := demoBaselineDeadline(ctx); err != nil {
				return fmt.Errorf("阶段9健康基线未在观察窗口内全部通过：%w", err)
			}
			all = all && passed
		}
		if err := demoBaselineDeadline(ctx); err != nil {
			return fmt.Errorf("阶段9健康基线未在观察窗口内全部通过：%w", err)
		}
		if all {
			return nil
		}
		if err := sources.wait(ctx, demoBaselineInterval); err != nil {
			return fmt.Errorf("阶段9健康基线未在观察窗口内全部通过：%w", err)
		}
	}
}

// demoBaselineDeadline还核对实际截止时刻，不能在context定时通知尚未调度时晚到放行。
func demoBaselineDeadline(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return nil
}
