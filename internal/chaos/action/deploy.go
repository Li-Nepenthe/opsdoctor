// 本文件：保存发布前的配置，并通过 Compose 发布与恢复服务。
//
// 配置改变会真正重建容器。恢复使用原镜像的内容 ID，避免同名标签后来被新构建覆盖。
// 只保存公开配置，不把容器里的密码复制进实验记录。
//
// 主要内容：
//   - Baseline、Deployment：记录恢复需要的信息。
//   - Capture：在任何改动前读取流量与相关服务的配置。
//   - deploy、Restore：执行发布，恢复时使用刚才的快照。
//
// 阅读建议：先看 Capture 的字段筛选，再看恢复为什么禁止重新构建旧镜像。

package action

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
)

// Baseline 是本机恢复记录的一部分，不含凭据。
type Baseline struct {
	Services map[string]Deployment `json:"services"`   // 只记录本场景会发布的服务。
	QPS      int                   `json:"qps"`        // 恢复操作者原先的流量，而不是强行设为默认值。
	HotSKUID int64                 `json:"hot_sku_id"` // 同时恢复选品分布。
}

// Deployment 用镜像内容 ID 和公开环境重建原服务。
type Deployment struct {
	Image       string            `json:"image"`       // docker inspect 返回 sha256 内容 ID。
	Environment map[string]string `json:"environment"` // 白名单参数，不含数据库地址或密码。
}

// Capture 在注入之前执行。无法可靠读取基线时不开始实验。
func (runner *Runner) Capture(ctx context.Context, value scenario.Scenario) (Baseline, error) {
	// 先读流量，再读取将要重建的容器；这时尚未执行任何注入动作。
	baseline := Baseline{Services: make(map[string]Deployment)}
	var control struct {
		Data struct {
			Settings *struct {
				QPS int   `json:"qps"`
				Hot int64 `json:"hot_sku_id"`
			} `json:"settings"`
		} `json:"data"`
	}
	if err := runner.request(ctx, "GET", runner.LoadURL+"/control", nil, &control); err != nil {
		return baseline, err
	}
	if control.Data.Settings == nil {
		return baseline, fmt.Errorf("流量控制接口没有返回实际设置")
	}
	baseline.QPS, baseline.HotSKUID = control.Data.Settings.QPS, control.Data.Settings.Hot
	for _, step := range value.Inject {
		if step.Action != "deploy" {
			continue
		}
		id, err := runner.compose(ctx, []string{"ps", "-q", step.Service}, nil)
		if err != nil {
			return baseline, err
		}
		if strings.TrimSpace(string(id)) == "" {
			return baseline, fmt.Errorf("待发布服务未运行：%s", step.Service)
		}
		data, err := runner.RunCommand(ctx, []string{"inspect", "--format", `{"image":{{json .Image}},"environment":{{json .Config.Env}}}`, strings.TrimSpace(string(id))}, nil)
		if err != nil {
			return baseline, err
		}
		var inspected struct {
			Image       string
			Environment []string
		}
		if err = json.Unmarshal(data, &inspected); err != nil {
			return baseline, err
		}
		if inspected.Image == "" {
			return baseline, fmt.Errorf("容器信息没有返回原镜像内容 ID")
		}
		saved := Deployment{Image: inspected.Image, Environment: make(map[string]string)}
		for _, entry := range inspected.Environment {
			// Docker 返回全部环境项，只把公开白名单留下，其余值不会落盘。
			key, value, _ := strings.Cut(entry, "=")
			if scenario.ValidateEnvironment(map[string]string{key: value}) == nil {
				saved.Environment[key] = value
			}
		}
		baseline.Services[step.Service] = saved
	}
	return baseline, nil
}

// deploy 用真实构建版本区分配置发布。恢复时复用旧镜像，不把旧版本重新编译成当前源码。
func (runner *Runner) deploy(ctx context.Context, step scenario.Step, previous *Deployment) error {
	// Compose 变量只对本次命令有效，不写回用户的 .env。
	values := make(map[string]string)
	for key, value := range step.Environment {
		values[key] = value
	}
	arguments := []string{"up", "-d", "--no-deps", "--wait", "--wait-timeout", "120"}
	imageVariable := "ORDER_IMAGE"
	if step.Service == "inventory-service" {
		imageVariable = "INVENTORY_IMAGE"
	}
	switch {
	case previous != nil:
		// 用内容 ID 恢复，避免原标签已经被实验中的构建覆盖。
		for key, value := range previous.Environment {
			values[key] = value
		}
		values[imageVariable] = previous.Image
		arguments = append(arguments, "--no-build")
	case step.Image != "":
		// 指定版本必须事先构建好，执行实验时不能意外拉取未知镜像。
		values[imageVariable] = step.Image
		arguments = append(arguments, "--no-build")
	default:
		// 配置发布也生成新的版本号，启动日志与 app_build_info 能对应这次变更。
		values["APP_VERSION"] = time.Now().UTC().Format("20060102.150405")
		values["ORDER_GO_TAGS"] = "" // 配置发布使用正常构建；另一版本只能通过明确镜像动作发布。
		arguments = append(arguments, "--build")
	}
	_, err := runner.compose(ctx, append(arguments, step.Service), values)
	return err
}

// Restore 对发布和流量使用注入前快照，其余动作仍按场景声明执行。
func (runner *Runner) Restore(ctx context.Context, step scenario.Step, baseline Baseline) error {
	if err := scenario.ValidateStep(step); err != nil {
		return err
	}
	if step.Action == "deploy" {
		previous, ok := baseline.Services[step.Service]
		if !ok {
			return fmt.Errorf("缺少 %s 的发布前记录", step.Service)
		}
		return runner.deploy(ctx, step, &previous)
	}
	if step.Action == "load" {
		step.QPS = &baseline.QPS
		step.HotSKUID = &baseline.HotSKUID
	}
	return runner.Execute(ctx, step)
}
