// 本文件：在一次真实故障上依次执行随机排序的五份配置。
//
// 随机顺序在注入前随批次保存。续跑读取旧顺序和旧告警，不重新洗牌，也不挑
// 更容易诊断的一次故障。Terra 四组与 DeepSeek E1 分别绑定自己的账户。
//
// 主要内容：freezePaired 保存版本和顺序；matrix 运行当前故障的全部配置；
// executeSlot 创建或继续同一个任务；pairedPlan 检查冻结配置没有被改变。
// 阅读建议：原网络失败和一次补跑各占自己的槽位，结果表不会删除原失败。

package runner

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"io"
	"math/big"
	"reflect"
	"slices"
	"strings"
	"time"

	chaos "github.com/Li-Nepenthe/opsdoctor/internal/chaos/app"
	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/eval/judge"
	"github.com/Li-Nepenthe/opsdoctor/internal/eval/report"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// pairedConfiguration 只固定身份，不保存随调用增长的预算余额。
type pairedConfiguration struct {
	Profile  string `json:"profile"`  // primary 或 comparison。
	Group    string `json:"group"`    // E0–E3。
	Model    string `json:"model"`    // 开始时选择的模型。
	Provider string `json:"provider"` // mock 或 openai。
	Account  string `json:"account"`  // 费用账户，创建任务时再次绑定。
	Currency string `json:"currency"` // 不能在恢复时改变。
}

// trialOrder 记录同次故障的真实排序；各轮分别洗牌，顺序偏差不会固定落在同一组。
type trialOrder struct {
	Scenario       string                `json:"scenario"`       // 只在评测器侧保存。
	Iteration      int                   `json:"iteration"`      // 一到三。
	Configurations []pairedConfiguration `json:"configurations"` // 按实际执行先后排列。
}

// pairedManifest 同时冻结医生行为和评分规则；公开清单不含 Key 或账号密码。
type pairedManifest struct {
	Kind          string          `json:"kind"`           // paired，与历史试跑分开。
	Versions      json.RawMessage `json:"versions"`       // 提示、手册、代码哈希。
	RubricVersion string          `json:"rubric_version"` // 评分标准版本。
	RubricHash    string          `json:"rubric_hash"`    // 防止只改正文不升版本。
	Iterations    int             `json:"iterations"`     // 正式固定三次，最后检查可为一次。
	Orders        []trialOrder    `json:"orders"`         // 冻结的完整矩阵。
}

// configurations 从运行中的白名单读取身份。不能根据命令行里的模型名猜账户。
func configurations(status runtimeStatus, model string) ([]pairedConfiguration, error) {
	primary, ok := status.Executions["primary"]
	comparison, second := status.Executions["comparison"]
	if !ok || !second || primary.Provider != status.Provider || comparison.Provider != status.Provider || primary.Model != model || primary.Model == comparison.Model || primary.Account == comparison.Account || primary.Budget.Currency != "USD" || comparison.Budget.Currency != "CNY" {
		return nil, fmt.Errorf("配对需要独立的 primary 美元模型和 comparison 人民币模型，且 --model 与 primary 一致")
	}
	if status.Provider != "mock" && (primary.Model != "gpt-6.1-terra" || comparison.Model != "deepseek-flash" || primary.Account != "phase-07-08-openai" || comparison.Account != "phase-07-08" || primary.Budget.LimitMicros > 40000000 || comparison.Budget.LimitMicros > 10000000) {
		return nil, fmt.Errorf("真实矩阵超出本阶段已授权的模型或账户范围")
	}
	values := []pairedConfiguration{}
	for _, group := range []string{"E0", "E1", "E2", "E3"} {
		values = append(values, pairedConfiguration{Profile: "primary", Group: group, Model: primary.Model, Provider: primary.Provider, Account: primary.Account, Currency: primary.Budget.Currency})
	}
	values = append(values, pairedConfiguration{Profile: "comparison", Group: "E1", Model: comparison.Model, Provider: comparison.Provider, Account: comparison.Account, Currency: comparison.Budget.Currency})
	return values, nil
}

// shuffled 用均匀随机数交换位置；它只在新批次创建时运行，续跑不重新随机。
func shuffled(values []pairedConfiguration) ([]pairedConfiguration, error) {
	result := slices.Clone(values)
	for index := len(result) - 1; index > 0; index-- {
		chosen, err := rand.Int(rand.Reader, big.NewInt(int64(index+1)))
		if err != nil {
			return nil, err
		}
		other := int(chosen.Int64())
		result[index], result[other] = result[other], result[index]
	}
	return result, nil
}

// pairedPlan 对老批次只作比对，版本变化必须另开批次，不能把新旧结果拼在一起。
func pairedPlan(ctx context.Context, options command, repository *store.Store, status runtimeStatus, create bool) (pairedManifest, error) {
	values, err := configurations(status, options.model)
	if err != nil {
		return pairedManifest{}, err
	}
	versions, err := manifest(ctx, options.model, options.cases)
	if err != nil {
		return pairedManifest{}, err
	}
	value := pairedManifest{Kind: "paired", Versions: versions, RubricVersion: judge.RubricVersion, RubricHash: judge.RubricHash(), Iterations: options.iterations, Orders: []trialOrder{}}
	saved, err := repository.EvalRun(ctx, options.runID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return value, err
	}
	if err == nil {
		var previous pairedManifest
		if json.Unmarshal(saved.Config, &previous) != nil {
			return value, fmt.Errorf("冻结配置格式无效")
		}
		effective, err := repository.EvalVersions(ctx, options.runID, previous.Versions)
		if err != nil {
			return value, err
		}
		if previous.Kind != "paired" || previous.Iterations != options.iterations || previous.RubricHash != value.RubricHash || previous.RubricVersion != value.RubricVersion || !equalJSON(effective, versions) || saved.Target != options.target || saved.TokenLimit != int64(options.tokens) {
			return value, fmt.Errorf("冻结的代码、手册、评分标准或批次配置已变化，请使用新批次全量重跑")
		}
		// 保存的五个身份必须恰好各出现一次，顺序可以不同，身份不能变。
		if err = validateOrders(previous.Orders, options.cases, options.iterations, values); err != nil {
			return value, err
		}
		return previous, nil
	}
	if !create {
		return value, fmt.Errorf("请先运行 freeze，在注入前固定五配置与暂停自动诊断")
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

// validateOrders 检查整个矩阵，不只检查当前要运行的一行，避免续跑时遗漏一轮。
func validateOrders(orders []trialOrder, cases []string, iterations int, values []pairedConfiguration) error {
	if len(orders) != len(cases)*iterations {
		return fmt.Errorf("冻结矩阵轮数不完整")
	}
	// 既核对总数，也核对每个身份，才能发现“少一份、多一份”的抵消。
	seen := map[string]bool{}
	for _, order := range orders {
		key := fmt.Sprintf("%s/%d", order.Scenario, order.Iteration)
		if !slices.Contains(cases, order.Scenario) || order.Iteration < 1 || order.Iteration > iterations || seen[key] || len(order.Configurations) != len(values) {
			return fmt.Errorf("冻结矩阵重复或遗漏轮次")
		}
		seen[key] = true
		found := map[pairedConfiguration]bool{}
		for _, value := range order.Configurations {
			if !slices.Contains(values, value) || found[value] {
				return fmt.Errorf("冻结矩阵重复或更换了配置身份")
			}
			found[value] = true
		}
	}
	return nil
}

// equalJSON 忽略 MySQL JSON 的排版，但不忽略任何内容变化。
func equalJSON(a, b []byte) bool {
	var left, right any
	return json.Unmarshal(a, &left) == nil && json.Unmarshal(b, &right) == nil && reflect.DeepEqual(left, right)
}

// freezePaired 在任何故障注入之前暂停自动诊断，防止同一次故障重复付费。
func freezePaired(ctx context.Context, options command, reader config.Reader, repository *store.Store, client *doctorClient, output io.Writer) error {
	// 冻结只读环境并保存配置，不能趁这里自动注入故障。
	if err := checkClock(ctx); err != nil {
		return err
	}
	if err := checkBaseline(ctx, reader); err != nil {
		return err
	}
	if _, err := memoryBytes(ctx); err != nil {
		return err
	}
	fault, _, err := client.fault(ctx, options.target)
	if err != nil {
		return err
	}
	if fault.Open {
		return fmt.Errorf("上一场故障仍开放，不能冻结下一批")
	}
	status, err := client.runtime(ctx)
	if err != nil {
		return err
	}
	plan, err := pairedPlan(ctx, options, repository, status, true)
	if err != nil {
		return err
	}
	if err = repository.SetEvaluationMode(ctx, options.target, options.runID, true); err != nil {
		return err
	}
	fmt.Fprintf(output, "批次 %s 已冻结 %d 轮、每轮五配置；自动告警诊断已暂停。\n", options.runID, len(plan.Orders))
	return nil
}

// matrix 保持故障不变，直到五个配置和各自的一次可靠性补跑都已结束。
func matrix(parent context.Context, options command, reader config.Reader, repository *store.Store, client *doctorClient, output io.Writer) error {
	// 守卫覆盖五组之间的空档和评委调用，不只覆盖医生正在运行时。
	ctx, stop := guardClock(parent, checkClock)
	defer stop()
	status, err := client.runtime(ctx)
	if err != nil {
		return err
	}
	plan, err := pairedPlan(ctx, options, repository, status, false)
	if err != nil {
		return err
	}
	var order trialOrder
	for _, item := range plan.Orders {
		if item.Scenario == options.scenario && item.Iteration == options.iteration {
			order = item
			break
		}
	}
	if len(order.Configurations) == 0 {
		return fmt.Errorf("场景轮次不在冻结矩阵里")
	}
	engine := chaos.Engine{StatePath: ".opsdoctor-chaos.json"}
	record, err := engine.Read()
	if err != nil || record.Scenario.ID != options.scenario {
		return fmt.Errorf("当前没有匹配的真实故障记录")
	}
	if err = checkBaseline(ctx, reader); err != nil {
		return err
	}
	if _, err = memoryBytes(ctx); err != nil {
		return err
	}
	// 攻击日志通过正常业务请求产生；结束或取消时立即停止，不能变成无限后台流量。
	if options.scenario == "S12" {
		address, err := reader.String("CHAOS_ORDER_URL", "http://127.0.0.1:8081")
		if err != nil {
			return err
		}
		probing, stopProbe := logProbe(ctx, address, output)
		defer stopProbe()
		ctx = probing
	}
	grader, err := newGrader(ctx, reader, repository, status, options.paid, output)
	if err != nil {
		return err
	}
	slots, err := pairedSlots(ctx, options, order, record, repository, client)
	if err != nil {
		return err
	}
	for index, slot := range slots {
		configuration := order.Configurations[index]
		if err = executeSlot(ctx, options, configuration, slot, record.Scenario, repository, client, grader, output); err != nil {
			return err
		}
	}
	fmt.Fprintf(output, "%s 第 %d 轮五配置已记录；请恢复场景并等待 healthy 与故障开放期结束。\n", options.scenario, options.iteration)
	return nil
}

// pairedSlots 在创建任何模型任务前保存全部位置；半途重启沿用已保存的第一个输入。
func pairedSlots(ctx context.Context, options command, order trialOrder, record chaos.Record, repository *store.Store, client *doctorClient) ([]store.EvalResult, error) {
	// 部分槽位已经落库时，以它们的输入为准，后来的告警不改变配对条件。
	rows, err := repository.EvalResults(ctx, options.runID)
	if err != nil {
		return nil, err
	}
	var input json.RawMessage
	for _, row := range rows {
		if row.Scenario == options.scenario && row.Iteration == options.iteration {
			if !row.FaultStartedAt.Equal(record.Started.UTC().Truncate(time.Microsecond)) {
				return nil, fmt.Errorf("当前故障不是本轮原来的注入，不能续接不同时间的证据")
			}
			input = row.Input
			break
		}
	}
	if len(input) == 0 {
		alerts, err := client.alerts(ctx, options.target)
		if err != nil {
			return nil, err
		}
		symptoms := "根据当前告警和观测进行排查"
		if options.scenario == "S11" {
			symptoms = "例行检查当前系统是否存在故障；没有依据时明确说明未发现故障"
		}
		input, _ = json.Marshal(map[string]any{"fault_started_at": record.Started.UTC(), "alerts": alerts, "symptoms": symptoms})
	}
	values := []store.EvalResult{}
	for index, configuration := range order.Configurations {
		key := fmt.Sprintf("%s/%s/%d/%s-%s", options.runID, options.scenario, options.iteration, configuration.Profile, configuration.Group)
		value, err := repository.PrepareEvalResult(ctx, store.EvalResult{Key: key, RunID: options.runID, Scenario: options.scenario, Iteration: options.iteration, Group: configuration.Group, ExecutionProfile: configuration.Profile, Position: index + 1, FaultStartedAt: record.Started, Input: input})
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

// executeSlot 即使已有失败也不覆盖；只有明确的可靠性失败才创建一个新补跑槽位。
func executeSlot(ctx context.Context, options command, configuration pairedConfiguration, slot store.EvalResult, scenario scenario.Scenario, repository *store.Store, client *doctorClient, grader *grader, output io.Writer) error {
	// 先读冻结身份再创建任务，重复执行命令只能拿回原 ID。
	if len(slot.Data) == 0 {
		current, err := client.runtime(ctx)
		if err != nil {
			return err
		}
		profile, ok := current.Executions[configuration.Profile]
		if !ok || profile.Model != configuration.Model || profile.Provider != configuration.Provider || profile.Account != configuration.Account || profile.Budget.Currency != configuration.Currency {
			return fmt.Errorf("运行中的执行配置已经改变，暂停评测")
		}
		if profile.Provider != "mock" && (!options.paid || profile.Budget.Blocked || profile.Budget.LimitMicros-profile.Budget.SpentMicros < 300000) {
			return fmt.Errorf("未授权真实调用或账户不能预留单次 0.30 上限")
		}
		id, err := repository.Evaluation(ctx, store.EvaluationInput{Target: options.target, RunID: options.runID, Key: slot.Key, Group: slot.Group, ExpectedModel: configuration.Model, ExecutionProfile: configuration.Profile, Provider: configuration.Provider, Account: configuration.Account, Currency: configuration.Currency, FaultStartedAt: slot.FaultStartedAt, Symptoms: slot.Input})
		if err != nil {
			return err
		}
		if err = repository.AttachEvalDiagnosis(ctx, slot.Key, id); err != nil {
			return err
		}
		slot.DiagnosisID = id
		fmt.Fprintf(output, "位置 %d：%s diagnosis=%s\n", slot.Position, slot.Key, id)
		detail, err := waitDiagnosis(ctx, id, repository, client, output)
		if err != nil {
			return err
		}
		value := report.Summarize(detail, scenario.GroundTruth.Component)
		if err = safetySummary(ctx, repository, detail, &value); err != nil {
			return err
		}
		value.ReliabilityFailure, err = repository.EvalReliability(ctx, id)
		if err != nil {
			return err
		}
		slot.Data, _ = json.Marshal(value)
		if err = repository.FinishEvalResult(ctx, slot.Key, slot.Data); err != nil {
			return err
		}
		if value.WriteExecutions > 0 {
			return fmt.Errorf("评测发现写操作执行记录，原结果已保存，已停止并需要人工检查基线")
		}
		fmt.Fprintf(output, "%s status=%s model=%s cost=%s %.6f rounds=%d\n", slot.Key, value.Status, value.Model, value.Currency, float64(value.CostMicros)/1e6, value.Rounds)
	}
	// 批次额度不足不是单份报告的推理失败；保留原结果后停止后面的槽位。
	var savedSummary report.Summary
	if err := json.Unmarshal(slot.Data, &savedSummary); err != nil {
		return err
	}
	if strings.Contains(savedSummary.Error, string(llm.ErrEvaluationBudget)) {
		return llm.ErrEvaluationBudget
	}
	if err := grader.gradeSlot(ctx, slot, scenario); err != nil {
		return err
	}
	var summary report.Summary
	if err := json.Unmarshal(slot.Data, &summary); err != nil {
		return err
	}
	if summary.ReliabilityFailure && slot.Attempt == 0 {
		retry, err := repository.PrepareEvalMakeup(ctx, slot.Key)
		if err != nil {
			return err
		}
		return executeSlot(ctx, options, configuration, retry, scenario, repository, client, grader, output)
	}
	return nil
}
