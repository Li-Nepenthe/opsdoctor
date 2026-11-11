// 本文件：命令行通过 Run 连接宿主机的实验流程。
// operate 校验命令与现有记录的对应关系，overrides 只更改本次运行的参数。

package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/action"
	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/verify"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// Run 不隐式启动流量。healthy 需要事先有真实流量，暂停状态不会被判成通过。
func Run(ctx context.Context, args []string, reader config.Reader, output io.Writer) error {
	// list 只读文件，不要求外部组件或高权限凭据已经准备好。
	if len(args) == 0 {
		return fmt.Errorf("用法：chaos list | inject ID [--qps N --sku ID --latency-ms N] | verify ID | restore ID | restore-all | healthy | selftest")
	}
	values, err := scenario.Load("eval/scenarios")
	if err != nil {
		return err
	}
	if args[0] == "list" {
		if len(args) != 1 {
			return fmt.Errorf("list 不接受额外参数")
		}
		for _, value := range values {
			fmt.Fprintf(output, "%s\t%s\n", value.ID, value.Title)
		}
		return nil
	}
	engine, err := newEngine(reader)
	if err != nil {
		return err
	}
	switch args[0] {
	case "selftest", "healthy", "restore-all":
		if len(args) != 1 {
			return fmt.Errorf("%s 不接受额外参数", args[0])
		}
		return engine.general(ctx, args[0], values, output)
	case "inject", "verify", "restore":
		if len(args) < 2 {
			return fmt.Errorf("%s 需要场景编号", args[0])
		}
		for _, value := range values {
			if value.ID == args[1] {
				return engine.operate(ctx, args[0], value, args[2:], output)
			}
		}
		return fmt.Errorf("不存在场景 %s", args[1])
	default:
		return fmt.Errorf("未知子命令 %s", args[0])
	}
}

// newEngine 只读取密码，不输出它。非索引场景可以不配置数据库账号。
func newEngine(reader config.Reader) (*Engine, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	order, orderErr := reader.String("CHAOS_ORDER_URL", "http://127.0.0.1:8081")
	proxy, proxyErr := reader.String("TOXIPROXY_URL", "http://127.0.0.1:8474")
	load, loadErr := reader.String("LOADGEN_CONTROL_URL", "http://127.0.0.1:6070")
	prometheus, promErr := reader.String("PROMETHEUS_URL", "http://127.0.0.1:9090")
	redis, redisErr := reader.String("CHAOS_REDIS_ADDR", "127.0.0.1:6379")
	products, productsErr := reader.Int("CHAOS_BASELINE_PRODUCT_COUNT", 10000, 1, 500000)
	workers, workersErr := reader.Int("CHAOS_BASELINE_WORKERS", 64, 1, 256)
	if err = errors.Join(orderErr, proxyErr, loadErr, promErr, redisErr, productsErr, workersErr); err != nil {
		return nil, err
	}
	// 只在索引动作真正需要连接时要求凭据。
	dsn, _ := reader.String("CHAOS_MYSQL_DSN", "")
	password, _ := reader.String("REDIS_PASSWORD", "")
	runner := &action.Runner{OrderURL: order, Root: root, RedisPassword: password, MySQLDSN: dsn, RedisAddress: redis, ProxyURL: proxy, LoadURL: load, HTTP: &http.Client{Timeout: 10 * time.Second}}
	runner.RunCommand = runner.Command
	baselineDSN, _ := reader.String("CHAOS_BASELINE_MYSQL_DSN", "")
	observer := verify.WithBaseline(verify.New(prometheus), load, products, workers).WithRepairs(baselineDSN)
	return &Engine{Executor: runner, Observer: observer, StatePath: filepath.Join(root, ".opsdoctor-chaos.json")}, nil
}

// general 处理无需场景编号的命令。restore-all 只恢复本工具记录的活动实验，不重置无关资源。
func (engine *Engine) general(ctx context.Context, command string, values []scenario.Scenario, output io.Writer) error {
	// 整套自测为每个场景单独计时；单独健康与恢复命令共用三分钟上限。
	if command == "selftest" {
		return engine.Selftest(ctx, values, output)
	}
	bounded, cancel := context.WithTimeout(ctx, phaseBudget)
	defer cancel()
	if command == "healthy" {
		result, err := engine.Observer.Healthy(bounded)
		fmt.Fprintf(output, "healthy elapsed=%s last=%s\n", result.Elapsed.Round(time.Millisecond), result.LastProblem)
		return err
	}
	record, err := engine.Read()
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(output, "没有本工具记录的活动实验")
		return nil
	}
	if err != nil {
		return err
	}
	return engine.Restore(bounded, record)
}

// operate 用已有记录匹配 verify 和 restore，防止把另一场景的效果误当成当前场景。
func (engine *Engine) operate(ctx context.Context, command string, value scenario.Scenario, args []string, output io.Writer) error {
	// 只有 inject 接受覆盖参数；verify 使用落盘的原场景，不能临时降低门槛。
	if command == "inject" {
		if err := overrides(&value, args, output); err != nil {
			return err
		}
		// Load 已检查这个值；注入和后续 verify 必须使用同一个场景预算。
		budget, _ := time.ParseDuration(value.Symptom.Within)
		bounded, cancel := context.WithTimeout(ctx, budget)
		defer cancel()
		_, err := engine.Inject(bounded, value)
		if err != nil {
			return fmt.Errorf("%w；已创建记录时请执行 restore-all", err)
		}
		fmt.Fprintf(output, "%s 动作已执行；请继续 verify，不能据此认定症状已出现\n", value.ID)
		return nil
	}
	if len(args) > 0 {
		return fmt.Errorf("%s 不接受额外参数", command)
	}
	record, err := engine.Read()
	if err != nil {
		return err
	}
	if record.Scenario.ID != value.ID {
		return fmt.Errorf("活动场景是 %s", record.Scenario.ID)
	}
	deadline := time.Now().Add(phaseBudget)
	if command == "verify" {
		budget, err := time.ParseDuration(record.Scenario.Symptom.Within)
		if err != nil || budget <= 0 || budget > phaseBudget {
			return fmt.Errorf("恢复记录中的观察时限不合法")
		}
		deadline = record.Started.Add(budget)
	}
	bounded, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if command == "restore" {
		return engine.Restore(bounded, record)
	}
	result, err := engine.Observer.Symptom(bounded, record.Scenario.Symptom)
	fmt.Fprintf(output, "%s symptom=%t alerts=%v elapsed=%s\n", value.ID, result.SymptomSeen, result.Alerts, time.Since(record.Started).Round(time.Millisecond))
	return err
}

// overrides 只覆盖对应动作已有的参数；没有这种动作的场景会拒绝无效的覆盖选项。
func overrides(value *scenario.Scenario, args []string, output io.Writer) error {
	options, err := parseOverrides(args, output, value.Inject)
	if err != nil {
		return err
	}
	usedQPS, usedSKU, usedLatency := false, false, false
	for index := range value.Inject {
		// 同一个热点编号同步用于流量和单商品缓存，避免删了 A 的缓存却压测 B。
		step := &value.Inject[index]
		if step.Action == "load" {
			if options.qps >= 0 {
				step.QPS = &options.qps
				usedQPS = true
			}
			if options.sku >= 0 {
				step.HotSKUID = &options.sku
				usedSKU = true
			}
		}
		if (step.Action == "delete_cache" || step.Action == "expire_cache") && step.Count == 1 && options.sku > 0 {
			step.FirstSKUID = options.sku
			usedSKU = true
		}
		if step.Action == "latency" && options.latencyMS >= 0 {
			step.LatencyMS = options.latencyMS
			usedLatency = true
		}
		if err := scenario.ValidateStep(*step); err != nil {
			return err
		}
	}
	if (options.qps >= 0 && !usedQPS) || (options.sku >= 0 && !usedSKU) || (options.latencyMS >= 0 && !usedLatency) {
		return fmt.Errorf("场景没有对应动作，覆盖参数未生效")
	}
	return nil
}

type overrideOptions struct {
	qps       int
	sku       int64
	latencyMS int
}

// 先确认命令行格式及选项与场景的兼容性，这一阶段遇到错误时保留原步骤。
func parseOverrides(args []string, output io.Writer, steps []scenario.Step) (overrideOptions, error) {
	var options overrideOptions
	// 未指定的选项用 -1 表示，零仍可表示暂停或恢复普通选品。
	flags := flag.NewFlagSet("inject", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.IntVar(&options.qps, "qps", -1, "覆盖流量动作的目标速率")
	flags.Int64Var(&options.sku, "sku", -1, "覆盖热点商品与单商品缓存操作的编号")
	flags.IntVar(&options.latencyMS, "latency-ms", -1, "覆盖网络延迟，单位毫秒")
	if err := flags.Parse(args); err != nil {
		return options, err
	}
	if flags.NArg() != 0 || options.qps < -1 || options.sku < -1 || options.latencyMS < -1 {
		return options, fmt.Errorf("实验参数不合法")
	}
	for _, step := range steps {
		// 商品突发必须先暂停普通流量，随后只发查询。通用热点覆盖会把下单也集中起来，重新混入行锁症状。
		if step.Action == "burst" && (options.qps >= 0 || options.sku >= 0) {
			return options, fmt.Errorf("商品突发场景使用固定的暂停、批次和商品范围，不接受通用 qps 或 sku 覆盖")
		}
	}
	return options, nil
}
