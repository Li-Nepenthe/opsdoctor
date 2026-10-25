// 本文件：把已检查的场景动作交给真正的外部系统执行。
//
// 注入器在宿主机运行。它能改靶场的结构和配置，因此使用独立账号，且不被业务服务导入。
// 每个动作只做一件事，实验顺序和失败恢复由 app 负责。
//
// 主要内容：
//   - Runner：持有外部地址和可替换的命令执行函数。
//   - Execute：校验动作后分派到数据库、网络、发布或流量操作。
//   - Command：直接启动 docker，不经过 shell 解释参数。
//
// 阅读建议：先看分派，再去对应文件查看真正发出的请求。

package action

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
)

// commandBudget 限制部署或建索引的最长等待；超时仍需恢复，不能推断动作没有发生。
const commandBudget = 3 * time.Minute

// Runner 只保存配置，不长时间占用数据库连接。测试可以替换 RunCommand 来观察命令。
type Runner struct {
	Root         string                                                             // 仓库根目录，Compose 的相对路径由此解释。
	MySQLDSN     string                                                             // 独立的 chaos 账号 DSN，绝不写进输出或本机恢复记录。
	RedisAddress string                                                             // 只操作 product: 前缀，禁止清库。
	ProxyURL     string                                                             // Toxiproxy 本机管理地址。
	LoadURL      string                                                             // loadgen 本机控制地址。
	HTTP         *http.Client                                                       // 所有控制请求都有超时。
	RunCommand   func(context.Context, []string, map[string]string) ([]byte, error) // 测试用假执行器替换 docker。
}

// Execute 再次检查参数，防止调用方绕过 YAML 加载器后执行任意操作。
func (runner *Runner) Execute(ctx context.Context, step scenario.Step) error {
	// 所有入口共用同一校验，直接调用执行器也不能绕过白名单。
	if err := scenario.ValidateStep(step); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, commandBudget)
	defer cancel()
	switch step.Action {
	case "drop_index", "restore_index":
		return runner.index(ctx, step)
	case "deploy":
		return runner.deploy(ctx, step, nil)
	case "stop", "start":
		_, err := runner.compose(ctx, []string{step.Action, step.Service}, nil)
		return err
	case "latency", "clear_latency":
		return runner.latency(ctx, step)
	case "load":
		return runner.load(ctx, step)
	case "delete_cache", "expire_cache":
		return runner.cache(ctx, step)
	default:
		return fmt.Errorf("没有实现动作 %s", step.Action)
	}
}

// Command 使用参数数组，YAML 里的值不会被 PowerShell 或 bash 当成代码。
func (runner *Runner) Command(ctx context.Context, args []string, values map[string]string) ([]byte, error) {
	command := exec.CommandContext(ctx, "docker", args...)
	command.Dir = runner.Root
	// 覆盖同名变量前先删除旧值，避免 Windows 与 Linux 对重复环境项选择不同值。
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, overridden := values[key]; !overridden {
			command.Env = append(command.Env, entry)
		}
	}
	for key, value := range values {
		command.Env = append(command.Env, key+"="+value)
	}
	output, err := command.CombinedOutput()
	if err != nil {
		// Compose 的原始诊断可能展开含密码的配置，不复制出来；操作者可单独查看 Docker 状态。
		return nil, fmt.Errorf("docker 命令未完成：%w（原始输出未回显，避免暴露环境变量）", err)
	}
	return output, nil
}

// compose 固定配置文件与项目目录，只接受内部代码构造的子命令。
func (runner *Runner) compose(ctx context.Context, args []string, values map[string]string) ([]byte, error) {
	command := append([]string{"compose", "--env-file", ".env", "-f", "deploy/docker-compose.yml"}, args...)
	return runner.RunCommand(ctx, command, values)
}
