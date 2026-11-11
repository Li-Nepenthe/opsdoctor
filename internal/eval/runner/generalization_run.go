// 本文件：推进第二个系统的真实实验，并为每次注入保留独立恢复文件。
//
// 数据库先记录“正在注入”，再写完整原开关文件，最后调用 Demo。中断后先比对
// 原文件和当前开关，不能重新注入一次更容易诊断的故障。恢复文件始终保留。
//
// 主要内容：runGeneralization 遍历冻结顺序；advanceDemoTrial 推进一个阶段；
// demoMatrix 复用原诊断与评分路径；restoreDemoRecord 提供不依赖数据库的恢复入口。
// 阅读建议：先看状态推进，再看恢复文件怎样核对同一次故障。

package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/eval/report"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// demoRecovery 不含密码；Original 和 Injected 是官方故障配置的完整快照。
type demoRecovery struct {
	RunID     string          `json:"run_id"`    // 每批独立，不能复用阶段 8 的恢复文件。
	Scenario  string          `json:"scenario"`  // 只在评测器本机保存。
	Iteration int             `json:"iteration"` // 本次注入的轮次。
	Flagd     string          `json:"flagd"`     // 与冻结端点相同才允许恢复。
	Started   time.Time       `json:"started"`   // 同组共用，续跑不刷新。
	Original  json.RawMessage `json:"original"`  // 注入前完整 flags。
	Injected  json.RawMessage `json:"injected"`  // 注入后应有的完整 flags。
}

// generalizationGuardKey 区分用户要求“异常即停”的阶段 9 与阶段 8 的自动可靠性补跑。
type generalizationGuardKey struct{}

// 仅阶段9延长真实症状与恢复等待；shop和数据库的五分钟冷却规则保持原样。
const (
	demoObservationLimit  = 5 * time.Minute
	demoJudgeFailureLimit = 3 // 本批第三份评分失败保留后暂停；前两份不重评。
)

// demoJudgeClient只观察原客户端的最终结果，不增加请求、重试或改变冻结评分。
type demoJudgeClient struct {
	llm.Client
	mock     bool // Mock没有HTTP账本，只用于免费流程检查。
	returned bool // 原有限重试结束后，是否取得了可确认的回答和计量。
}

// Complete保留原Response/error原样返回；有错误时即使带正文也不能当作格式失败豁免。
func (client *demoJudgeClient) Complete(ctx context.Context, request llm.Request) (llm.Response, error) {
	response, err := client.Client.Complete(ctx, request)
	client.returned = err == nil && demoJudgeResponse(response, client.mock)
	return response, err
}

// demoJudgeResponse确认外层回答存在；内层参数是否合法仍由冻结Check判断。
func demoJudgeResponse(response llm.Response, mock bool) bool {
	if response.Model == "" || response.Message.Role != "assistant" || response.CostMicros < 0 {
		return false
	}
	return mock || response.UsageKnown && len(response.AttemptIDs) > 0
}

// checkGeneralizationSummary 保存失败后才拦截后续收费；原阶段 8 仍按既有规则补跑。
func checkGeneralizationSummary(ctx context.Context, value report.Summary, key string) error {
	if ctx.Value(generalizationGuardKey{}) != nil && (value.Status != "succeeded" || value.MixedModel || value.ReliabilityFailure) {
		return fmt.Errorf("阶段 9 诊断执行异常或中途换模型，原结果已保留，暂停后续付费：%s", key)
	}
	return nil
}

// demoRecoveryPath 每轮使用独立文件；实验结束也不删除，失败和恢复证据都能留下。
func demoRecoveryPath(options generalizationCommand) (string, error) {
	if !regexp.MustCompile(`^phase09-[A-Za-z0-9_-]{1,56}$`).MatchString(options.runID) || !regexp.MustCompile(`^OT0[1-8]$`).MatchString(options.scenario) || options.iteration < 1 || options.iteration > 3 {
		return "", fmt.Errorf("恢复记录的批次或场景轮次不合法")
	}
	return filepath.Join(os.TempDir(), "opsdoctor-phase09", "recovery", options.runID, fmt.Sprintf("%s-%d.json", options.scenario, options.iteration)), nil
}

// readDemoRecord 同时核对文件身份和数据库故障时刻，避免接上另一天的同名实验。
func readDemoRecord(options generalizationCommand, trial store.EvalTrial) (demoRecovery, error) {
	var value demoRecovery
	// 文件名只是第一层身份；文件内容和数据库时间还要再次精确对应。
	path, err := demoRecoveryPath(options)
	if err != nil {
		return value, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	if len(raw) > 2<<20 || json.Unmarshal(raw, &value) != nil || value.RunID != options.runID || value.Scenario != options.scenario || value.Iteration != options.iteration || value.Flagd != options.flagd || value.Started.IsZero() || !json.Valid(value.Original) || !json.Valid(value.Injected) || (trial.FaultStartedAt != nil && !trial.FaultStartedAt.Equal(value.Started.UTC().Truncate(time.Microsecond))) {
		return value, fmt.Errorf("恢复文件与本轮身份或开始时间不符")
	}
	return value, nil
}

// createDemoRecord 使用独占创建，原文件存在时拒绝覆盖；写入关闭成功后才允许注入。
func createDemoRecord(options generalizationCommand, value demoRecovery) error {
	// O_EXCL 拒绝覆盖，Sync 和 Close 都成功后调用方才可以修改 Demo。
	path, err := demoRecoveryPath(options)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("不能独占创建恢复文件，原记录未覆盖：%w", err)
	}
	raw, _ := json.MarshalIndent(value, "", "  ")
	_, writeErr := file.Write(raw)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	return errors.Join(writeErr, file.Close())
}

// runGeneralization 只运行已冻结的批次；异常留档后停止，原开关记录留待明确恢复。
func runGeneralization(parent context.Context, options generalizationCommand, cases map[string]demoScenario, reader config.Reader, repository *store.Store, client *doctorClient, output io.Writer) (problem error) {
	// 时钟守卫覆盖整个批次，包含等待冷却和评委正在收费的时间。
	if err := checkClock(parent); err != nil {
		return err
	}
	ctx, stopClock := guardClock(parent, checkClock)
	defer stopClock()
	ctx = context.WithValue(ctx, diagnosisMemoryKey{}, demoMemoryBytes)
	ctx = context.WithValue(ctx, generalizationGuardKey{}, true)
	ctx = withDemoTiming(ctx) // 原time.Time只留在本次进程，续跑不能伪造已丢失的单调锚点。
	status, err := client.runtime(ctx)
	if err != nil {
		return err
	}
	plan, err := generalizationPlan(ctx, options, repository, status, false)
	if err != nil {
		return err
	}
	options.baseline = plan.Baseline
	options.versions = plan.Versions
	if status.Provider != "mock" && !options.paid {
		return fmt.Errorf("真实运行需要 --allow-paid，freeze 本身不会开启收费")
	}
	_, mode, err := client.fault(ctx, options.target)
	if err != nil || !mode.Enabled || mode.RunID != options.runID {
		return fmt.Errorf("阶段 9 批次没有持有本系统的评测模式：%v", err)
	}
	if err = recordDemoEvaluator(ctx, options, repository, "run", ""); err != nil {
		return err
	}
	if err = repository.SetEvalRunState(ctx, options.runID, "running"); err != nil {
		return err
	}
	defer func() {
		// 进程退出后仍保留评测模式，防止自动告警趁中断再创建付费诊断。
		if problem != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
			defer cancel()
			problem = errors.Join(problem, repository.SetEvalRunState(cleanup, options.runID, "interrupted"))
		}
	}()
	// 续跑先检查本批原失败；先装好interrupt收尾，再拒绝第三份失败或API异常。
	if err = checkDemoSlot(ctx, options, "", repository); err != nil {
		return err
	}
	for _, order := range plan.Orders {
		options.scenario, options.iteration = order.Scenario, order.Iteration
		if err = runDemoTrial(ctx, options, cases, order, reader, repository, client, output); err != nil {
			return err
		}
	}
	if err = repository.SetEvalRunState(ctx, options.runID, "completed"); err != nil {
		return err
	}
	return finish(ctx, options.command, repository, client, output)
}

// runDemoTrial 每一步都先读取数据库；中断状态不会被新命令悄悄重置为 pending。
func runDemoTrial(ctx context.Context, options generalizationCommand, cases map[string]demoScenario, order trialOrder, reader config.Reader, repository *store.Store, client *doctorClient, output io.Writer) error {
	// 每次推进之前重新读状态，数据库已完成的步骤不能由命令重试再次执行。
	for {
		trial, err := repository.EvalTrial(ctx, options.runID, options.scenario, options.iteration)
		if err != nil || trial.State == "done" {
			return err
		}
		fmt.Fprintf(output, "Demo %s/%d state=%s\n", trial.Scenario, trial.Iteration, trial.State)
		if err = recordDemoEvaluator(ctx, options, repository, trial.State, ""); err != nil {
			return err
		}
		if err = advanceDemoTrial(ctx, options, cases, order, trial, reader, repository, client, output); err != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if ctx.Err() != nil {
				err = context.Cause(ctx)
			}
			return errors.Join(err, repository.InterruptEval(cleanup, trial, err.Error()))
		}
	}
}

// advanceDemoTrial 复用原 trial 状态机，只有外部注入、验证和恢复手段不同。
func advanceDemoTrial(ctx context.Context, options generalizationCommand, cases map[string]demoScenario, order trialOrder, trial store.EvalTrial, reader config.Reader, repository *store.Store, client *doctorClient, output io.Writer) error {
	value := cases[options.scenario]
	// 与外部系统之间没有分布式事务，所以先写 injecting，再依靠快照确认动作结果。
	switch trial.State {
	case "pending":
		if err := demoBaseline(ctx, options, cases, client); err != nil {
			return err
		}
		if err := repository.AdvanceEvalTrial(ctx, trial, "injecting", nil); err != nil {
			return err
		}
		return injectDemoRecord(ctx, options, value, output)
	case "injecting", "verifying", "diagnosing":
		record, err := activeDemoRecord(ctx, options, trial)
		if err != nil {
			return err
		}
		if trial.State == "injecting" {
			return repository.AdvanceEvalTrial(ctx, trial, "verifying", &record.Started)
		}
		if trial.State == "verifying" {
			within, _ := time.ParseDuration(value.Symptom.Within)
			if err = waitDemoQueryObserved(ctx, options.prometheus, value.Symptom.PromQL, record.Started.Add(within), func(at time.Time, polls int) error {
				return observeDemoSymptom(ctx, options, at, polls)
			}); err != nil {
				return err
			}
			return repository.AdvanceEvalTrial(ctx, trial, "diagnosing", nil)
		}
		if err = demoMatrix(ctx, options, value, order, record, reader, repository, client, output); err != nil {
			return err
		}
		return repository.AdvanceEvalTrial(ctx, trial, "restoring", nil)
	case "restoring":
		if err := restoreDemoRecord(ctx, options, output); err != nil {
			return err
		}
		return repository.AdvanceEvalTrial(ctx, trial, "cooling", nil)
	case "cooling":
		return coolDemoTrial(ctx, options, cases, trial, repository, client)
	default:
		return fmt.Errorf("未知 Demo 实验状态 %s", trial.State)
	}
}

// injectDemoRecord 完整保存原文件后才发修改；写入结果不确定时下一次只核对，不重注入。
func injectDemoRecord(ctx context.Context, options generalizationCommand, value demoScenario, output io.Writer) error {
	// 健康检查与写开关之间仍可能有人改配置；写之前再对照冻结基线。
	original, err := readDemoFlags(ctx, options.flagd)
	if err != nil {
		return err
	}
	if len(options.baseline) > 0 && !equalJSON(original, options.baseline) {
		return fmt.Errorf("注入前开关已偏离冻结健康配置，暂停实验")
	}
	injected, err := changeDemoFlag(original, value)
	if err != nil {
		return err
	}
	if equalJSON(original, injected) {
		return fmt.Errorf("故障开关已经处在目标状态，不能把旧故障当成新注入")
	}
	at := time.Now()
	if err = rememberDemoInjection(ctx, options, at); err != nil {
		return err
	}
	record := demoRecovery{RunID: options.runID, Scenario: options.scenario, Iteration: options.iteration, Flagd: options.flagd, Started: at.UTC().Truncate(time.Microsecond), Original: original, Injected: injected}
	if err = createDemoRecord(options, record); err != nil {
		return err
	}
	path, _ := demoRecoveryPath(options)
	fmt.Fprintf(output, "原始开关已保存：%s；现在注入。\n", path)
	return writeDemoFlags(ctx, options.flagd, injected)
}

// activeDemoRecord 不只检查自己写过文件，还要求当前全部开关仍与本轮注入一致。
func activeDemoRecord(ctx context.Context, options generalizationCommand, trial store.EvalTrial) (demoRecovery, error) {
	// 恢复文件证明本轮的意图，当前读回结果证明本轮故障仍然有效。
	value, err := readDemoRecord(options, trial)
	if err != nil {
		return value, fmt.Errorf("注入结果不确定，不能自动重做：%w", err)
	}
	path, _ := demoRecoveryPath(options)
	if err = checkDemoUnrestored(path); err != nil {
		return value, err
	}
	actual, err := readDemoFlags(ctx, options.flagd)
	if err != nil {
		return value, err
	}
	if !equalJSON(actual, value.Injected) {
		return value, fmt.Errorf("外部靶场 Demo 当前开关与本次注入不同，暂停核对")
	}
	return value, nil
}

// checkDemoUnrestored 恢复确认一旦存在就不能接续旧注入，即使有人重新打开了同名开关。
func checkDemoUnrestored(path string) error {
	_, err := os.Stat(path + ".restored")
	if err == nil {
		return fmt.Errorf("本轮已有恢复确认，不能把后来重新开启的故障接到旧诊断")
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("无法核对本轮恢复确认：%w", err)
	}
	return nil
}

// restoreDemoRecord 只恢复与原快照或本轮注入相同的状态，避免覆盖他人中途改过的开关。
func restoreDemoRecord(ctx context.Context, options generalizationCommand, output io.Writer) error {
	// 原配置和本轮配置之外的状态可能来自人，不自动覆盖这种未知变更。
	value, err := readDemoRecord(options, store.EvalTrial{})
	if err != nil {
		return err
	}
	actual, err := readDemoFlags(ctx, options.flagd)
	if err != nil {
		return err
	}
	if !equalJSON(actual, value.Original) && !equalJSON(actual, value.Injected) {
		return fmt.Errorf("当前开关存在本轮以外的修改，停止自动恢复，请人工核对原快照")
	}
	if !equalJSON(actual, value.Original) {
		if err = writeDemoFlags(ctx, options.flagd, value.Original); err != nil {
			return err
		}
		if options.action != "restore" {
			// 只记录正常轮次真实写后读回的时刻；紧急恢复不依赖计时设施。
			if err = rememberDemoRestore(ctx, options, time.Now()); err != nil {
				return err
			}
		}
	}
	path, _ := demoRecoveryPath(options)
	if err = writeDemoRestoreProof(path, value.Original); err != nil {
		return err
	}
	fmt.Fprintf(output, "Demo 原始开关已恢复并读回确认；恢复文件保留：%s\n", path)
	return nil
}

// writeDemoRestoreProof 另写确认文件，原快照永不截断；崩溃也不会丢掉唯一的恢复依据。
func writeDemoRestoreProof(path string, original json.RawMessage) error {
	// 证明与原始字节哈希绑定；重复恢复可验证旧证明，但不能改写它。
	hash := sha256.Sum256(original)
	proof := struct {
		Hash string    `json:"original_sha256"`
		At   time.Time `json:"confirmed_at"`
	}{Hash: hex.EncodeToString(hash[:]), At: time.Now().UTC()}
	proofPath := path + ".restored"
	file, err := os.OpenFile(proofPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, os.ErrExist) {
		prior, readErr := os.ReadFile(proofPath)
		var saved struct {
			Hash string `json:"original_sha256"`
		}
		if readErr != nil || len(prior) > 1024 || json.Unmarshal(prior, &saved) != nil || saved.Hash != proof.Hash {
			return fmt.Errorf("原快照保留，但恢复确认文件需要人工核对")
		}
		return nil
	}
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(proof)
	_, writeErr := file.Write(raw)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	return errors.Join(writeErr, file.Close())
}

// recordDemoEvaluator 每个动作前核对冻结版本；长跑中改文件也会在下一收费前停下。
func recordDemoEvaluator(ctx context.Context, options generalizationCommand, repository *store.Store, operation, key string) error {
	versions, err := manifest(ctx, options.model, options.cases)
	if err != nil {
		return err
	}
	if len(options.versions) == 0 || !equalJSON(options.versions, versions) {
		return fmt.Errorf("阶段 9 冻结代码、场景、说明书或手册已改变，暂停本批次")
	}
	return repository.RecordEvalExecution(ctx, store.EvalExecution{
		RunID: options.runID, Scenario: options.scenario, Iteration: options.iteration,
		Operation: operation, ResultKey: key, Versions: versions,
	})
}

// coolDemoTrial 用数据库的恢复时间等五分钟，窗口结束后再检查全部服务基线。
func coolDemoTrial(ctx context.Context, options generalizationCommand, cases map[string]demoScenario, trial store.EvalTrial, repository *store.Store, client *doctorClient) error {
	value := cases[options.scenario]
	// 先看到健康，再等数据库记录的冷却窗口结束，旧日志不会马上污染下一次实验。
	if err := waitDemoQueryObserved(ctx, options.prometheus, value.HealthyPromQL, time.Now().Add(demoObservationLimit), func(at time.Time, polls int) error {
		return observeDemoRecovery(ctx, options, at, polls)
	}); err != nil {
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
	if err := demoBaselinePhase(ctx, options, cases, client, "post-cool"); err != nil {
		return err
	}
	return repository.AdvanceEvalTrial(ctx, trial, "done", nil)
}

// demoMatrix 先保存所有槽位与同一份症状，再调用现有执行器和冻结评委。
func demoMatrix(parent context.Context, options generalizationCommand, value demoScenario, order trialOrder, record demoRecovery, reader config.Reader, repository *store.Store, client *doctorClient, output io.Writer) (problem error) {
	// 诊断和评分期间持续核对全部开关；有人改动时取消当前付费请求。
	ctx, stop, err := guardLoad(parent, func(checkContext context.Context) error {
		_, err := activeDemoRecord(checkContext, options, store.EvalTrial{FaultStartedAt: &record.Started})
		return err
	})
	if err != nil {
		return err
	}
	defer stop()
	defer func() {
		if problem != nil && ctx.Err() != nil {
			problem = context.Cause(ctx)
		}
	}()
	status, err := client.runtime(ctx)
	if err != nil {
		return err
	}
	grader, err := newGraderForAccount(ctx, reader, repository, output, judgeExecution{
		account: "phase-09-deepseek", status: status, paid: options.paid,
	})
	if err != nil {
		return err
	}
	input, err := demoSlotInput(ctx, options, value, record, repository, client)
	if err != nil {
		return err
	}
	slots := []store.EvalResult{}
	// 所有组先占住固定位置，再运行模型，重启时不会因为输入改变而重新抽样。
	for index, configuration := range order.Configurations {
		key := fmt.Sprintf("%s/%s/%d/%s-%s", options.runID, options.scenario, options.iteration, configuration.Profile, configuration.Group)
		slot, err := repository.PrepareEvalResult(ctx, store.EvalResult{Key: key, RunID: options.runID, Scenario: options.scenario, Iteration: options.iteration, Group: configuration.Group, ExecutionProfile: configuration.Profile, Position: index + 1, FaultStartedAt: record.Started, Input: input})
		if err != nil {
			return err
		}
		slots = append(slots, slot)
	}
	for index, slot := range slots {
		if _, err = activeDemoRecord(ctx, options, store.EvalTrial{FaultStartedAt: &record.Started}); err != nil {
			return err
		}
		if _, err = demoMemoryBytes(ctx); err != nil {
			return err
		}
		if err = recordDemoEvaluator(ctx, options, repository, "slot", slot.Key); err != nil {
			return err
		}
		if err = executeSlot(ctx, options.command, order.Configurations[index], slot, value.gradingScenario(), repository, client, grader, output); err != nil {
			return err
		}
		if err = checkDemoSlot(ctx, options, slot.Key, repository); err != nil {
			return err
		}
	}
	return nil
}

// demoSlotInput 续跑优先取原槽位；新日志和后来的告警不会偷偷换掉配对输入。
func demoSlotInput(ctx context.Context, options generalizationCommand, value demoScenario, record demoRecovery, repository *store.Store, client *doctorClient) (json.RawMessage, error) {
	// 已有任何一组结果时，以那次最初输入为准，不刷新成更容易诊断的快照。
	rows, err := repository.EvalResults(ctx, options.runID)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.Scenario == options.scenario && row.Iteration == options.iteration {
			if !row.FaultStartedAt.Equal(record.Started.UTC().Truncate(time.Microsecond)) {
				return nil, fmt.Errorf("已有槽位不属于本次注入")
			}
			return row.Input, nil
		}
	}
	alerts, err := client.alerts(ctx, options.target)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"fault_started_at": record.Started, "alerts": alerts, "symptoms": value.Symptoms})
}

// checkDemoSlot读取本批原记录；格式评分失败可保留缺失，第三份或API异常仍暂停。
func checkDemoSlot(ctx context.Context, options generalizationCommand, key string, repository *store.Store) error {
	rows, err := repository.EvalResults(ctx, options.runID)
	if err != nil {
		return err
	}
	return checkDemoJudgements(options.runID, key, rows)
}

// checkDemoJudgements不调用评委也不改原文；累计只认同一run保存的judgement。
func checkDemoJudgements(runID, key string, rows []store.EvalResult) error {
	failures := 0
	matched := key == "" // 空key用于续跑前检查，允许尚未创建任何槽位的新批次。
	for _, row := range rows {
		if row.RunID != runID {
			continue
		}
		current := key != "" && (row.Key == key || row.RetryOf == key)
		matched = matched || current
		if len(row.Data) == 0 && len(row.Judgement) == 0 && !current {
			continue // 已占位置但尚未诊断的槽位，不是假定已经成功。
		}
		var summary report.Summary
		if json.Unmarshal(row.Data, &summary) != nil || summary.Status != "succeeded" || summary.ReliabilityFailure || summary.MixedModel {
			return fmt.Errorf("阶段 9 诊断执行异常，原结果已保留，暂停后续付费：%s", row.Key)
		}
		if len(row.Judgement) == 0 && !current {
			continue // 中断的首次评分由原gradeSlot恢复原回答，不在这里重问。
		}
		var grade gradedResult
		if row.JudgeID == "" || json.Unmarshal(row.Judgement, &grade) != nil || grade.Status != "ok" && grade.Status != "failed" {
			return fmt.Errorf("阶段9缺少可核对的原评分，暂停后续付费：%s", row.Key)
		}
		if grade.ResponseReturned == nil || !*grade.ResponseReturned {
			return fmt.Errorf("阶段9评分API失败或返回/费用无法确认，原记录保留，暂停后续付费：%s", row.Key)
		}
		if grade.Status == "failed" {
			failures++ // 有返回但冻结Check拒收，保持缺失；不把默认零值当作分数。
		}
	}
	if !matched {
		return fmt.Errorf("阶段9原评分槽位不存在，暂停后续付费")
	}
	if failures >= demoJudgeFailureLimit {
		return fmt.Errorf("阶段9本批累计%d份评分失败，原结果与费用保留，暂停后续付费", failures)
	}
	return nil
}
