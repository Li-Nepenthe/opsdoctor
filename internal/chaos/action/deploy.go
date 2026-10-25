// 本文件：保存发布前的配置，并通过 Compose 发布与恢复服务。
//
// 配置改变会真正重建容器。先给原镜像留一个本地标签，再记录内容 ID，避免原引用被新构建覆盖。
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
	Image       string            `json:"image"`       // 保留原镜像的本地标签，后续构建不覆盖它。
	ImageID     string            `json:"image_id"`    // 恢复前核对内容，不能只相信标签名称。
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
		// 只记 ID 不会替 Docker 保留镜像。新构建覆盖原标签、旧容器又被移除后，原 ID 可能已无法使用。
		// 在任何发布动作之前加一个独立标签，让旧内容有明确引用。重复保存同一内容会复用同一标签。
		savedTag := "opsdoctor/" + step.Service + ":saved-" + strings.TrimPrefix(inspected.Image, "sha256:")
		if _, err = runner.RunCommand(ctx, []string{"image", "tag", inspected.Image, savedTag}, nil); err != nil {
			return baseline, fmt.Errorf("保留原镜像：%w", err)
		}
		saved := Deployment{Image: savedTag, ImageID: inspected.Image, Environment: make(map[string]string)}
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
		// 恢复前核对保存标签仍指向原内容。禁止拉取或重建，避免悄悄换成另一份程序。
		actual, err := runner.RunCommand(ctx, []string{"image", "inspect", "--format", "{{.Id}}", previous.Image}, nil)
		if err != nil {
			return err
		}
		if previous.ImageID == "" || strings.TrimSpace(string(actual)) != previous.ImageID {
			return fmt.Errorf("保存的镜像内容与原记录不一致，停止恢复")
		}
		for key, value := range previous.Environment {
			values[key] = value
		}
		values[imageVariable] = previous.Image
		arguments = append(arguments, "--no-build", "--pull", "never")
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
