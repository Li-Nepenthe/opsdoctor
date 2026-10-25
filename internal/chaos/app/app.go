// 本文件：把 chaos 命令行接到场景、执行器和自动验收流程。
//
// 程序从仓库根目录运行。普通业务服务不会调用这里，也不会拿到这里的高权限账号。
// 命令错误会返回非零退出码，方便把完整实验结果保存到文件。
//
// 主要内容：
//   - Run：读取场景，装配外部适配器，分派子命令。
//   - operate：执行单个场景的注入、观察或恢复。
//   - overrides：检查显式实验参数，不改写仓库中的 YAML。
//
// 阅读建议：先运行 list，再对照单个场景的 inject 和 restore 顺序。

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
	proxy, proxyErr := reader.String("TOXIPROXY_URL", "http://127.0.0.1:8474")
	load, loadErr := reader.String("LOADGEN_CONTROL_URL", "http://127.0.0.1:6070")
	prometheus, promErr := reader.String("PROMETHEUS_URL", "http://127.0.0.1:9090")
	redis, redisErr := reader.String("CHAOS_REDIS_ADDR", "127.0.0.1:6379")
	if err = errors.Join(proxyErr, loadErr, promErr, redisErr); err != nil {
		return nil, err
	}
	// 缺少 DSN 时只在真正执行索引动作时报错，list、healthy 等只读命令不需要高权限凭据。
	dsn, _ := reader.String("CHAOS_MYSQL_DSN", "")
	runner := &action.Runner{Root: root, MySQLDSN: dsn, RedisAddress: redis, ProxyURL: proxy, LoadURL: load, HTTP: &http.Client{Timeout: 10 * time.Second}}
	runner.RunCommand = runner.Command
	return &Engine{Executor: runner, Observer: verify.New(prometheus), StatePath: filepath.Join(root, ".opsdoctor-chaos.json")}, nil
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
		bounded, cancel := context.WithTimeout(ctx, phaseBudget)
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
		deadline = record.Started.Add(phaseBudget)
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
	// -1 只表示没有传入选项，真正的零值仍可以表达暂停或解除热点。
	flags := flag.NewFlagSet("inject", flag.ContinueOnError)
	flags.SetOutput(output)
	qps := flags.Int("qps", -1, "覆盖流量动作的目标速率")
	sku := flags.Int64("sku", -1, "覆盖热点商品与单商品缓存操作的编号")
	latency := flags.Int("latency-ms", -1, "覆盖网络延迟，单位毫秒")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *qps < -1 || *sku < -1 || *latency < -1 {
		return fmt.Errorf("实验参数不合法")
	}
	usedQPS, usedSKU, usedLatency := false, false, false
	for index := range value.Inject {
		// 同一个热点编号同步用于流量和单商品缓存，避免删了 A 的缓存却压测 B。
		step := &value.Inject[index]
		if step.Action == "load" {
			if *qps >= 0 {
				step.QPS = qps
				usedQPS = true
			}
			if *sku >= 0 {
				step.HotSKUID = sku
				usedSKU = true
			}
		}
		if (step.Action == "delete_cache" || step.Action == "expire_cache") && step.Count == 1 && *sku > 0 {
			step.FirstSKUID = *sku
			usedSKU = true
		}
		if step.Action == "latency" && *latency >= 0 {
			step.LatencyMS = *latency
			usedLatency = true
		}
		if err := scenario.ValidateStep(*step); err != nil {
			return err
		}
	}
	if (*qps >= 0 && !usedQPS) || (*sku >= 0 && !usedSKU) || (*latency >= 0 && !usedLatency) {
		return fmt.Errorf("场景没有对应动作，覆盖参数未生效")
	}
	return nil
}
