// 本文件：为 OpenTelemetry Demo 提供独立批次入口，不借用 shop 的注入或账户。
//
// freeze 只保存版本和随机组序；run 才执行已授权的真实诊断。两者使用同一份冻结
// 清单，续跑不能更换场景、模型、端点或评分标准。export 仅导出已有数据。
//
// 主要内容：runGeneralizationCLI 解析并装配；generalizationPlan 固定批次；
// generalizationConfigurations 核对阶段 9 的两个独立账户。
// 阅读建议：从参数校验开始读，再进入 generalization_run.go 的状态推进。

package runner

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/eval/judge"
	"github.com/Li-Nepenthe/opsdoctor/internal/eval/report"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/go-sql-driver/mysql"
)

// generalizationCommand 只接受公开实验参数；Key 和密码仍只通过环境变量读取。
type generalizationCommand struct {
	command                                      // 复用槽位、诊断等待与评分所需的安排。
	groups                       []string        // 阶段 9 只运行 E1、E2。
	directory, flagd, prometheus string          // 场景目录与本机独立 Demo 端点。
	baseline                     json.RawMessage // 冻结时保存的原始开关，只在本机评测器传递。
	versions                     json.RawMessage // 每次动作和收费之前与冻结版本重新比对。
}

// generalizationManifest 保存完整顺序和端点，模型身份与评分版本都在注入前固定。
type generalizationManifest struct {
	Kind          string          `json:"kind"`               // 与阶段 8 的 paired/pilot 区分。
	Versions      json.RawMessage `json:"versions"`           // 包含提示、手册、场景和全部 profile 哈希。
	RubricVersion string          `json:"rubric_version"`     // 沿用 rubric-v1.1。
	RubricHash    string          `json:"rubric_hash"`        // 防止正文变化但版本号不变。
	Iterations    int             `json:"iterations"`         // 同场景重复次数，1–3。
	Flagd         string          `json:"flagd"`              // 宿主机注入端点，不给医生。
	Prometheus    string          `json:"prometheus"`         // 只读验证端点。
	Directory     string          `json:"scenario_directory"` // 冻结场景所在目录。
	Orders        []trialOrder    `json:"orders"`             // 每次注入的随机组序。
	Baseline      json.RawMessage `json:"baseline_flags"`     // 注入前的完整配置，用于发现中途改动。
}

// parseGeneralization 在读取数据库或模型配置前拒绝非法参数，拼错命令不会花钱。
func parseGeneralization(args []string, output io.Writer) (generalizationCommand, error) {
	value := generalizationCommand{}
	// 先检查操作名，再注册各个参数；无效输入在读取凭据之前就会结束。
	if len(args) == 0 || !slices.Contains([]string{"freeze", "run", "export", "finish", "restore"}, args[0]) {
		return value, fmt.Errorf("用法：eval generalization freeze|run|export|finish|restore --run phase09-批次 [--scenarios OT01,OT02 --groups E1 --allow-paid]")
	}
	value.action = args[0]
	flags := flag.NewFlagSet("eval generalization "+value.action, flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&value.runID, "run", "", "阶段 9 独立批次，必须以 phase09- 开头")
	flags.StringVar(&value.target, "target", "otel-demo", "已注册的第二个系统")
	flags.StringVar(&value.model, "model", "gpt-6.1-terra", "运行中的 primary 模型")
	flags.StringVar(&value.directory, "scenario-dir", "eval/scenarios/otel-demo", "外部系统场景目录")
	flags.StringVar(&value.flagd, "flagd-url", "http://127.0.0.1:18080", "官方 flagd UI 根地址")
	flags.StringVar(&value.prometheus, "prometheus-url", "http://127.0.0.1:19090", "Demo Prometheus 根地址")
	flags.StringVar(&value.scenario, "scenario", "", "仅用于紧急恢复的场景编号")
	flags.IntVar(&value.iterations, "iterations", 1, "冻结的每场景次数，1–3")
	flags.IntVar(&value.iteration, "iteration", 1, "紧急恢复的轮次")
	flags.BoolVar(&value.paid, "allow-paid", false, "运行已经获得用户授权的真实额度")
	cases := flags.String("scenarios", "OT01,OT02,OT03,OT04,OT05", "明确选择的外部场景")
	groups := flags.String("groups", "E1,E2", "E1 或 E1,E2，试跑使用 E1")
	if err := flags.Parse(args[1:]); err != nil {
		return value, err
	}
	if flags.NArg() != 0 || !regexp.MustCompile(`^phase09-[A-Za-z0-9_-]{1,56}$`).MatchString(value.runID) || value.target != "otel-demo" || value.iterations < 1 || value.iterations > 3 || value.iteration < 1 || value.iteration > 3 {
		return value, fmt.Errorf("阶段 9 批次、系统名或轮次不合法")
	}
	value.cases, value.groups = strings.Split(*cases, ","), strings.Split(*groups, ",")
	// 文件路径与编号一起收紧，才能保证执行的场景确实参与了源码哈希。
	seen := map[string]bool{}
	for _, name := range value.cases {
		if !regexp.MustCompile(`^OT0[1-8]$`).MatchString(name) || seen[name] {
			return value, fmt.Errorf("外部场景必须为不重复的 OT01–OT08")
		}
		seen[name] = true
	}
	if filepath.ToSlash(filepath.Clean(value.directory)) != "eval/scenarios/otel-demo" {
		return value, fmt.Errorf("阶段 9 场景目录固定为 eval/scenarios/otel-demo，确保实际场景纳入源码哈希")
	}
	if !slices.Equal(value.groups, []string{"E1"}) && !slices.Equal(value.groups, []string{"E1", "E2"}) {
		return value, fmt.Errorf("阶段 9 只允许 E1 或 E1,E2")
	}
	var err error
	value.flagd, err = demoBase(value.flagd)
	if err == nil {
		value.prometheus, err = demoBase(value.prometheus)
	}
	return value, err
}

// runGeneralizationCLI 的恢复入口不依赖数据库；数据库故障时仍能恢复 Demo 开关。
func runGeneralizationCLI(ctx context.Context, args []string, reader config.Reader, output io.Writer) error {
	ctx = context.WithValue(ctx, demoClockKey{}, true)
	// 紧急恢复不依赖数据库和医生登录，否则数据库停机时连恢复都做不了。
	options, err := parseGeneralization(args, output)
	if err != nil {
		return err
	}
	if options.action == "restore" {
		return restoreDemoRecord(ctx, options, output)
	}
	options.tokens, err = evalTokenBudget(reader)
	if err != nil {
		return err
	}
	repository, closeDatabase, err := generalizationStore(ctx, reader)
	if err != nil {
		return err
	}
	defer closeDatabase()
	// 导出只读已有结果，不重新创建槽位，也不再次向评委付费。
	if options.action == "export" {
		batch, err := repository.EvalRun(ctx, options.runID)
		if err != nil || batch.Target != "otel-demo" {
			return fmt.Errorf("阶段 9 批次不存在或绑定了其他系统：%v", err)
		}
		rows, err := repository.EvalResults(ctx, options.runID)
		if err != nil {
			return err
		}
		return report.Markdown(output, batch, rows)
	}
	client, err := newDoctorClient(ctx, reader)
	if err != nil {
		return err
	}
	if options.action == "finish" {
		return finish(ctx, options.command, repository, client, output)
	}
	cases, err := loadGeneralizationCases(options.directory, options.cases)
	if err != nil {
		return err
	}
	return repository.WithEvalLock(ctx, options.runID, func(locked context.Context) error {
		if options.action == "freeze" {
			return freezeGeneralization(locked, options, cases, repository, client, output)
		}
		return runGeneralization(locked, options, cases, reader, repository, client, output)
	})
}

// generalizationStore 仅连接医生自己的 doctor 库；导出和实验都不持有靶场数据库权限。
func generalizationStore(ctx context.Context, reader config.Reader) (*store.Store, func(), error) {
	// 评测器只有医生库账号；业务数据库的读写不从这里发生。
	dsn, err := reader.String("DOCTOR_MYSQL_DSN", "")
	if err != nil {
		return nil, nil, err
	}
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil || parsed.DBName != "doctor" || parsed.User != "doctor" {
		return nil, nil, fmt.Errorf("eval 只连接 doctor 库的 doctor 账号")
	}
	database, err := mysqlx.Open(ctx, mysqlx.Config{DSN: dsn, MaxOpenConns: 3, MaxIdleConns: 1, ConnMaxLifetime: time.Minute})
	if err != nil {
		return nil, nil, fmt.Errorf("无法连接医生数据库")
	}
	secrets := []string{}
	// 与医生发送模型请求相同，评委的审计也要清除本进程已知的凭据。
	for _, entry := range os.Environ() {
		name, value, _ := strings.Cut(entry, "=")
		if strings.HasSuffix(name, "_API_KEY") || strings.HasSuffix(name, "_PASSWORD") || strings.HasSuffix(name, "_TOKEN") {
			secrets = append(secrets, value)
		}
	}
	repository := store.NewConfigured(database, store.Config{Redactor: llm.NewRedactor(secrets...)})
	return repository, func() { _ = database.Close() }, nil
}

// generalizationConfigurations 严格核对新账户；旧阶段 8 账户即使有余额也不能复用。
func generalizationConfigurations(status runtimeStatus, model string, groups []string) ([]pairedConfiguration, error) {
	primary, first := status.Executions["primary"]
	comparison, second := status.Executions["comparison"]
	if status.Provider == "mock" && first && primary.Provider == "mock" && primary.Model == model && primary.Account == "phase-09-openai" && primary.Budget.Currency == "USD" && primary.Budget.LimitMicros > 0 && primary.Budget.LimitMicros <= 10000000 {
		// 运行中的Mock医生只需primary；评委使用本地judge.Mock，不要求伪造comparison模型。
		values := []pairedConfiguration{}
		for _, group := range groups {
			values = append(values, pairedConfiguration{Profile: "primary", Group: group, Model: primary.Model, Provider: "mock", Account: primary.Account, Currency: "USD"})
		}
		return values, nil
	}
	if (status.Provider != "mock" && status.Provider != "openai") || !first || !second || primary.Provider != status.Provider || comparison.Provider != status.Provider || primary.Model != model || primary.Account != "phase-09-openai" || comparison.Account != "phase-09-deepseek" || primary.Budget.Currency != "USD" || comparison.Budget.Currency != "CNY" || primary.Budget.LimitMicros <= 0 || comparison.Budget.LimitMicros <= 0 || primary.Budget.LimitMicros > 10000000 || comparison.Budget.LimitMicros > 10000000 {
		return nil, fmt.Errorf("阶段 9 需要独立 USD/CNY 账户，各自上限不能超过 10")
	}
	if status.Provider != "mock" && (primary.Model != "gpt-6.1-terra" || comparison.Model != "deepseek-flash") {
		return nil, fmt.Errorf("阶段 9 当前只授权 Terra 医生和 DeepSeek 评委")
	}
	values := []pairedConfiguration{}
	for _, group := range groups {
		values = append(values, pairedConfiguration{Profile: "primary", Group: group, Model: primary.Model, Provider: primary.Provider, Account: primary.Account, Currency: "USD"})
	}
	return values, nil
}

// generalizationPlan 创建时随机排序，续跑时逐项核对；不能覆盖原批次或原失败。
func generalizationPlan(ctx context.Context, options generalizationCommand, repository *store.Store, status runtimeStatus, create bool) (generalizationManifest, error) {
	// 先核对身份和行为版本，再看数据库里是否已经保存了不可覆盖的安排。
	values, err := generalizationConfigurations(status, options.model, options.groups)
	if err != nil {
		return generalizationManifest{}, err
	}
	versions, err := manifest(ctx, options.model, options.cases)
	if err != nil {
		return generalizationManifest{}, err
	}
	value := generalizationManifest{Kind: "generalization", Versions: versions, RubricVersion: judge.RubricVersion, RubricHash: judge.RubricHash(), Iterations: options.iterations, Flagd: options.flagd, Prometheus: options.prometheus, Directory: options.directory, Orders: []trialOrder{}}
	saved, err := repository.EvalRun(ctx, options.runID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return value, err
	}
	if err == nil {
		// 续跑只能沿用原顺序；即使新的随机顺序看起来等价，也不能替换原记录。
		var previous generalizationManifest
		if json.Unmarshal(saved.Config, &previous) != nil || !json.Valid(previous.Baseline) || previous.Kind != value.Kind || !equalJSON(previous.Versions, value.Versions) || previous.RubricVersion != value.RubricVersion || previous.RubricHash != value.RubricHash || previous.Iterations != value.Iterations || previous.Flagd != value.Flagd || previous.Prometheus != value.Prometheus || previous.Directory != value.Directory || saved.Target != options.target || saved.TokenLimit != int64(options.tokens) {
			return value, fmt.Errorf("阶段 9 冻结配置已变化，不能把新版本混入旧批次")
		}
		if err = validateOrders(previous.Orders, options.cases, options.iterations, values); err != nil {
			return value, err
		}
		return previous, nil
	}
	if !create {
		return value, fmt.Errorf("请先 generalization freeze 固定新批次")
	}
	value.Baseline, err = readDemoFlags(ctx, options.flagd)
	// 保存完整开关，而非只保存这次选中的五个，才能发现额外背景故障。
	if err != nil {
		return value, err
	}
	for _, name := range options.cases {
		for iteration := 1; iteration <= options.iterations; iteration++ {
			order, err := shuffled(values)
			if err != nil {
				return value, err
			}
			value.Orders = append(value.Orders, trialOrder{Scenario: name, Iteration: iteration, Configurations: order})
		}
	}
	raw, _ := json.Marshal(value)
	err = repository.CreateEvalRun(ctx, store.EvalRun{ID: options.runID, Target: options.target, Config: raw, TokenLimit: int64(options.tokens)})
	return value, err
}

// freezeGeneralization 在注入之前暂停该系统自动诊断，并记录尚未花钱的冻结清单。
func freezeGeneralization(ctx context.Context, options generalizationCommand, cases map[string]demoScenario, repository *store.Store, client *doctorClient, output io.Writer) error {
	// 冻结没有模型调用，但仍要求在真实健康系统上记录初始状态。
	if err := checkClock(ctx); err != nil {
		return err
	}
	if err := demoBaseline(ctx, options, cases, client); err != nil {
		return err
	}
	status, err := client.runtime(ctx)
	if err != nil {
		return err
	}
	plan, err := generalizationPlan(ctx, options, repository, status, true)
	if err != nil {
		return err
	}
	if err = repository.SetEvaluationMode(ctx, options.target, options.runID, true); err != nil {
		return err
	}
	fmt.Fprintf(output, "阶段 9 批次 %s 已冻结：%d 次注入，组 %s；自动诊断已暂停，本命令没有调用模型。\n", options.runID, len(plan.Orders), strings.Join(options.groups, ","))
	return nil
}
