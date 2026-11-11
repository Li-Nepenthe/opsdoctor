// 本文件：发布前由 Capture 保存服务原镜像与公开配置。
// Restore 依据内容 ID 核对保存的镜像，避免原来的标签已经被新构建覆盖。

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
	Services map[string]Deployment `json:"services"` // 只记录本场景会发布的服务。
	QPS      int                   `json:"qps"`      // 恢复操作者原先的流量，而不是强行设为默认值。
	HotSKUID int64                 `json:"hot_sku_id"`
}

// Deployment 用镜像内容 ID 和公开环境重建原服务。
type Deployment struct {
	Image       string            `json:"image"`       // 保留原镜像的本地标签，后续构建不覆盖它。
	ImageID     string            `json:"image_id"`    // 恢复前核对内容，不能只相信标签名称。
	Environment map[string]string `json:"environment"` // 白名单参数，不含数据库地址或密码。
}

// Capture 在注入之前执行。无法可靠读取基线时不开始实验。
func (runner *Runner) Capture(ctx context.Context, value scenario.Scenario) (Baseline, error) {
	// 先读取实际流量，再保存需要恢复的服务配置。
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
		// 仅记内容 ID 不能保留原镜像。发布前须为旧内容留下独立的本地标签。
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
