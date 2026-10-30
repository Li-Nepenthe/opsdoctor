// 本文件：提供阶段 8 第一停点之前的 E1 试跑命令。
//
// prepare 先暂停自动诊断；操作者注入并验证真实故障以后，pilot 才创建付费任务。
// 报告保存后仍需 chaos restore 和 healthy。finish 看到故障开放期结束才解除暂停。
// freeze/matrix 是用户确认试跑之后的配对入口，必须使用单独批次。
//
// 主要内容：RunCLI 装配依赖；prepare 固定版本；pilot 保存槽位和任务；
// waitDiagnosis 有界等待并处理取消；finish 等待系统开放期结束。
// 阅读建议：命令重试先取回旧槽位，绝不换一份告警输入或创建第二条诊断。

package runner

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	chaos "github.com/Li-Nepenthe/opsdoctor/internal/chaos/app"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/eval/report"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/clockcheck"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/go-sql-driver/mysql"
)

// command 只含公开运行安排；Key、数据库密码和登录令牌不接受命令行传值。
type command struct {
	interruption                           int64  // 只用于用户批准的、尚未创建诊断的基线中断重启。
	reason, makeupKey                      string // 校时修订的用户授权与原中断编号，不接受自动推断。
	action, runID, target, model, scenario string
	cases                                  []string
	paid                                   bool
	tokens                                 int
	iterations, iteration                  int // 批次重复次数与当前轮次，正式每个场景三次。
}

// RunCLI 先解析命令再连接依赖，拼错参数不会触发任何模型任务。
func RunCLI(ctx context.Context, args []string, reader config.Reader, output io.Writer) error {
	ctx = context.WithValue(ctx, clockMonitorKey{}, clockcheck.NewMonitor(recordClockEvent))
	// 校时检查不用数据库、登录令牌或模型 Key，环境故障时也能单独运行。
	// 只输出秒级时间戳；失败时沿用正式评测的拦截原因。
	if len(args) > 0 && args[0] == "check-clock" {
		if len(args) != 1 {
			return fmt.Errorf("用法：eval check-clock（不接受其他参数）")
		}
		return inspectClock(ctx, output)
	}
	// 试跑和配对分别命名；同一个批次不能混用两种运行安排。
	if len(args) == 0 || !slices.Contains([]string{"prepare", "pilot", "finish", "export", "freeze", "matrix", "run", "calibrate", "amend-clock", "amend-baseline"}, args[0]) {
		return fmt.Errorf("用法：eval check-clock；或 eval prepare|pilot|finish|export|freeze|matrix|run|calibrate|amend-clock|amend-baseline --run 批次 [--model 模型 --scenario 场景 --allow-paid]")
	}
	options := command{action: args[0]}
	flags := flag.NewFlagSet("eval "+args[0], flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&options.runID, "run", "", "持久化批次编号")
	flags.Int64Var(&options.interruption, "interruption", 0, "用户批准重启的原中断编号")
	flags.StringVar(&options.reason, "reason", "", "用户批准基础设施调整的具体原因")
	flags.StringVar(&options.makeupKey, "makeup-key", "", "明确允许补跑的原中断槽位")
	flags.StringVar(&options.target, "target", "shop", "已经接入的系统")
	flags.StringVar(&options.model, "model", "mock-script-v1", "明确指定医生模型")
	flags.StringVar(&options.scenario, "scenario", "", "已经注入并验证的场景")
	flags.IntVar(&options.iterations, "iterations", 3, "冻结批次的每场景次数，1 到 3")
	flags.IntVar(&options.iteration, "iteration", 1, "本次运行的轮次")
	flags.BoolVar(&options.paid, "allow-paid", false, "本次允许使用已授权的真实模型额度")
	cases := flags.String("scenarios", "S01,S03,S10", "本批两至三个 E1 试跑场景")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || options.runID == "" {
		return fmt.Errorf("请指定 --run，且不要提供额外位置参数")
	}
	options.cases = strings.Split(*cases, ",")
	paired := options.action == "freeze" || options.action == "matrix" || options.action == "run"
	if options.iterations < 1 || options.iterations > 3 || options.iteration < 1 || options.iteration > options.iterations {
		return fmt.Errorf("次数须为 1 到 3，当前轮次不能超出批次")
	}
	if !paired && (len(options.cases) < 2 || len(options.cases) > 3) {
		return fmt.Errorf("第一停点前只安排两至三个试跑场景")
	}
	seen := map[string]bool{}
	for _, name := range options.cases {
		if !slices.Contains([]string{"S01", "S02", "S03", "S04", "S05", "S06", "S07", "S08", "S09", "S10", "S11", "S12", "S13"}, name) || seen[name] {
			return fmt.Errorf("试跑清单必须是不重复的已实现故障场景")
		}
		seen[name] = true
	}
	var err error
	options.tokens, err = reader.Int("EVAL_TOKEN_BUDGET", 10000000, 1, 1000000000)
	if err != nil {
		return err
	}
	dsn, err := reader.String("DOCTOR_MYSQL_DSN", "")
	if err != nil {
		return err
	}
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil || parsed.DBName != "doctor" || parsed.User != "doctor" {
		return fmt.Errorf("eval 只连接 doctor 库的 doctor 账号")
	}
	database, err := mysqlx.Open(ctx, mysqlx.Config{DSN: dsn, MaxOpenConns: 3, MaxIdleConns: 1, ConnMaxLifetime: time.Minute})
	if err != nil {
		return fmt.Errorf("无法连接医生数据库")
	}
	defer database.Close()
	// 命令行的评委也使用同一份已知凭据清理规则，任何原文都不能从评分出口漏出。
	secrets := []string{}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if strings.HasSuffix(key, "_API_KEY") || strings.HasSuffix(key, "_PASSWORD") || strings.HasSuffix(key, "_TOKEN") {
			secrets = append(secrets, value)
		}
	}
	repository := store.NewConfigured(database, store.Config{Redactor: llm.NewRedactor(secrets...)})
	if options.action == "amend-clock" {
		return repository.WithEvalLock(ctx, options.runID, func(locked context.Context) error { return amendClock(locked, options, repository, output) })
	}
	if options.action == "export" {
		run, err := repository.EvalRun(ctx, options.runID)
		if err != nil {
			return err
		}
		rows, err := repository.EvalResults(ctx, options.runID)
		if err != nil {
			return err
		}
		return report.Markdown(output, run, rows)
	}
	if options.action == "calibrate" {
		return calibrate(ctx, repository, options.runID, output)
	}
	client, err := newDoctorClient(ctx, reader)
	if err != nil {
		return err
	}
	switch options.action {
	case "amend-baseline":
		return repository.WithEvalLock(ctx, options.runID, func(locked context.Context) error {
			return amendBaseline(locked, options, reader, repository, client, output)
		})
	case "freeze":
		return repository.WithEvalLock(ctx, options.runID, func(locked context.Context) error {
			return freezePaired(locked, options, reader, repository, client, output)
		})
	case "run":
		return repository.WithEvalLock(ctx, options.runID, func(locked context.Context) error {
			return automatic(locked, options, reader, repository, client, output)
		})
	case "matrix":
		return repository.WithEvalLock(ctx, options.runID, func(locked context.Context) error { return matrix(locked, options, reader, repository, client, output) })
	case "prepare":
		return prepare(ctx, options, reader, repository, client, output)
	case "pilot":
		return pilot(ctx, options, reader, repository, client, output)
	case "finish":
		return finish(ctx, options, repository, client, output)
	}
	return fmt.Errorf("未知操作")
}

// prepare 先核对健康环境，再保存版本和暂停状态；此命令本身不会调用付费模型。
func prepare(ctx context.Context, options command, reader config.Reader, repository *store.Store, client *doctorClient, output io.Writer) error {
	if err := checkClock(ctx); err != nil {
		return err
	}
	// 先检查真实环境，再落库。未通过检查时不能留下一个看起来已经开始的批次。
	if err := checkBaseline(ctx, reader); err != nil {
		return err
	}
	memory, err := memoryBytes(ctx)
	if err != nil {
		return err
	}
	fault, mode, err := client.fault(ctx, options.target)
	if err != nil {
		return err
	}
	if fault.Open && (!mode.Enabled || mode.RunID != options.runID) {
		return fmt.Errorf("上一场故障仍开放，请先恢复并等观察期结束")
	}
	configuration, err := manifest(ctx, options.model, options.cases)
	if err != nil {
		return err
	}
	if err = repository.CreateEvalRun(ctx, store.EvalRun{ID: options.runID, Target: options.target, Config: configuration, TokenLimit: int64(options.tokens)}); err != nil {
		return err
	}
	if err = repository.SetEvaluationMode(ctx, options.target, options.runID, true); err != nil {
		return err
	}
	fmt.Fprintf(output, "批次 %s 已准备；自动告警诊断暂停；实际内存 %.3f GiB。请先用 chaos 注入并验证，再运行 pilot。\n", options.runID, float64(memory)/(1<<30))
	return nil
}

// pilot 不负责注入，防止第一停点之前误启动正式矩阵。它只接收当前唯一的真实故障。
func pilot(ctx context.Context, options command, reader config.Reader, repository *store.Store, client *doctorClient, output io.Writer) error {
	// 输入和结果以槽位编号去重；重试命令不是多跑一次、挑一个更好的答案。
	if !slices.Contains(options.cases, options.scenario) {
		return fmt.Errorf("场景不在本批两至三个试跑清单内")
	}
	configuration, err := manifest(ctx, options.model, options.cases)
	if err != nil {
		return err
	}
	if err = repository.CreateEvalRun(ctx, store.EvalRun{ID: options.runID, Target: options.target, Config: configuration, TokenLimit: int64(options.tokens)}); err != nil {
		return err
	}
	key := options.runID + "/" + options.scenario + "/1/E1"
	rows, err := repository.EvalResults(ctx, options.runID)
	if err != nil {
		return err
	}
	var slot store.EvalResult
	for _, row := range rows {
		if row.Key == key {
			slot = row
			break
		}
	}
	if len(slot.Data) > 0 {
		fmt.Fprintf(output, "%s 已有终态结果，未重复创建或调用模型。\n", key)
		return nil
	}
	status, err := client.runtime(ctx)
	if err != nil {
		return err
	}
	if status.Model != options.model {
		return fmt.Errorf("运行模型 %s 与 --model %s 不符，停止试跑", status.Model, options.model)
	}
	if status.Provider != "mock" && (!options.paid || status.Budget.Blocked || status.Budget.LimitMicros-status.Budget.SpentMicros < 300000) {
		return fmt.Errorf("真实调用未显式启用，或费用账户不足以保留单次 0.30 上限")
	}
	if _, err = memoryBytes(ctx); err != nil {
		return err
	}
	if err = checkClock(ctx); err != nil {
		return err
	}
	engine := chaos.Engine{StatePath: filepath.Join(".", ".opsdoctor-chaos.json")}
	record, err := engine.Read()
	if err != nil || record.Scenario.ID != options.scenario {
		return fmt.Errorf("没有匹配本次场景的恢复记录，请先注入并验证")
	}
	want, err := scenarioLoad(options.scenario)
	if err != nil {
		return err
	}
	loadContext, stopLoad, err := guardLoad(ctx, func(checkContext context.Context) error { return checkLoad(checkContext, reader, want) })
	if err != nil {
		return err
	}
	defer stopLoad()
	ctx = loadContext
	if slot.Key != "" && !slot.FaultStartedAt.Equal(record.Started.UTC().Truncate(time.Microsecond)) {
		return fmt.Errorf("当前故障已不是原来的那次注入，不能把旧诊断拼进新实验")
	}
	if slot.Key == "" {
		captured, err := client.alerts(ctx, options.target)
		if err != nil {
			return err
		}
		input, _ := json.Marshal(map[string]any{"fault_started_at": record.Started.UTC(), "alerts": captured})
		slot, err = repository.PrepareEvalResult(ctx, store.EvalResult{Key: key, RunID: options.runID, Scenario: options.scenario, Iteration: 1, Group: "E1", Position: 1, FaultStartedAt: record.Started, Input: input})
		if err != nil {
			return err
		}
	}
	id, err := repository.Evaluation(ctx, store.EvaluationInput{Target: options.target, RunID: options.runID, Key: key, Group: "E1", ExpectedModel: options.model, FaultStartedAt: slot.FaultStartedAt, Symptoms: slot.Input})
	if err != nil {
		return err
	}
	if err = repository.AttachEvalDiagnosis(ctx, key, id); err != nil {
		return err
	}
	fmt.Fprintf(output, "%s diagnosis=%s；等待医生，所有费用经过原账户。\n", key, id)
	detail, err := waitDiagnosis(ctx, id, repository, client, output)
	if err != nil {
		return err
	}
	summary := report.Summarize(detail, record.Scenario.GroundTruth.Component)
	summary.ReliabilityFailure, err = repository.EvalReliability(ctx, id)
	if err != nil {
		return err // 审计读取失败时不猜测类别，也不发起补跑。
	}
	raw, _ := json.Marshal(summary)
	if err = repository.FinishEvalResult(ctx, key, raw); err != nil {
		return err
	}
	fmt.Fprintf(output, "status=%s rounds=%d tools=%d tokens=%d/%d cost=%s %.6f seconds=%.3f report_mode=%s；请恢复故障并运行 healthy。\n", summary.Status, summary.Rounds, summary.Tools, summary.InputTokens, summary.OutputTokens, summary.Currency, float64(summary.CostMicros)/1e6, summary.Seconds, summary.ReportMode)
	return nil
}

// waitDiagnosis 停止等待时也请求取消，不能让命令行退出后留下一条无人看管的付费请求。
func waitDiagnosis(parent context.Context, id string, repository *store.Store, client *doctorClient, output io.Writer) (detail store.Detail, problem error) {
	guarded, stopClock := guardClock(parent, checkClock)
	defer stopClock()
	// 等待器比诊断自身的五分钟预算略长，给排队和终态落库留出时间。
	ctx, cancel := context.WithTimeout(guarded, 7*time.Minute)
	defer cancel()
	// 时钟守卫可能在数据库轮询期间取消请求。保留具体的跳变原因，
	// 不让底层查询返回的 context cancelled 把需要用户处理的问题藏起来。
	defer func() {
		if problem != nil && ctx.Err() != nil {
			problem = context.Cause(ctx)
		}
	}()
	done := false
	defer func() {
		if !done {
			cleanup, stop := context.WithTimeout(context.WithoutCancel(parent), 3*time.Second)
			defer stop()
			_ = client.cancelDiagnosis(cleanup, id)
		}
	}()
	lastStatus := ""
	nextMemory := time.Now()
	for {
		state, err := repository.EvalDiagnosisState(ctx, id)
		if err != nil {
			return store.Detail{}, err
		}
		if state != lastStatus {
			fmt.Fprintf(output, "diagnosis=%s status=%s\n", id, state)
			lastStatus = state
		}
		if !slices.Contains([]string{"pending", "running", "waiting_approval"}, state) {
			done = true
			return repository.Detail(ctx, id)
		}
		if !time.Now().Before(nextMemory) {
			if _, err = memoryBytes(ctx); err != nil {
				return store.Detail{}, err
			}
			nextMemory = time.Now().Add(15 * time.Second)
		}
		select {
		case <-ctx.Done():
			return store.Detail{}, context.Cause(ctx)
		case <-time.After(2 * time.Second):
		}
	}
}

// finish 从真实接口读取开放期，未结束时说明截止时间；不自动批准操作或缩短观察期。
func finish(ctx context.Context, options command, repository *store.Store, client *doctorClient, output io.Writer) error {
	fault, _, err := client.fault(ctx, options.target)
	if err != nil {
		return err
	}
	if fault.Open {
		return fmt.Errorf("故障仍开放，观察期预计结束时间：%v；请恢复并等待", fault.RecoveryUntil)
	}
	if err = repository.SetEvaluationMode(ctx, options.target, options.runID, false); err != nil {
		return err
	}
	fmt.Fprintln(output, "评测模式已退出，自动告警诊断恢复；真实验收结束后还需将本机和运行中的医生切回 Mock。")
	return nil
}
