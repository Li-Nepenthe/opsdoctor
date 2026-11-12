// 本文件：从压测控制接口读取正在生效的参数，并用 Docker 的实际读数核对本 Compose 项目六 GiB 的内存暂停线。
// 基线不符就停止评测，保留原负载和告警阈值，其他项目的容器不计入。

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// checkBaseline 只发送无凭据的 GET，不把医生登录令牌带到压测器。
func checkBaseline(ctx context.Context, reader config.Reader) error {
	return checkLoad(ctx, reader, normalLoad())
}

// memoryBytes 在试跑和等待中重复检查；不根据容器上限之和猜测实际占用。
func memoryBytes(parent context.Context) (int64, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	ids, err := exec.CommandContext(ctx, "docker", "compose", "--env-file", ".env", "-f", "deploy/docker-compose.yml", "ps", "-q").Output()
	if err != nil {
		return 0, fmt.Errorf("无法读取本项目容器，请检查 Docker 引擎")
	}
	containers := strings.Fields(string(ids))
	if len(containers) == 0 {
		return 0, fmt.Errorf("本项目没有运行中的容器")
	}
	args := append([]string{"stats", "--no-stream", "--format", "{{json .}}"}, containers...)
	raw, err := exec.CommandContext(ctx, "docker", args...).Output()
	if err != nil {
		return 0, fmt.Errorf("docker stats 未完成")
	}
	var total int64
	// 少一个容器的数据也不能当作零，否则暂停线会低估实际占用。
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != len(containers) {
		return 0, fmt.Errorf("容器内存样本不完整")
	}
	for _, line := range lines {
		var sample struct {
			Memory string `json:"MemUsage"`
		}
		if json.Unmarshal([]byte(line), &sample) != nil {
			return 0, fmt.Errorf("容器内存响应无法解析")
		}
		size, err := memorySize(strings.TrimSpace(strings.SplitN(sample.Memory, "/", 2)[0]))
		if err != nil {
			return 0, err
		}
		total += size
	}
	if total > 6<<30 {
		return total, fmt.Errorf("本项目容器实际内存 %.3f GiB 超过用户指定的 6 GiB 暂停线，请停止并等待用户决定", float64(total)/(1<<30))
	}
	return total, nil
}

// memorySize 按 docker stats 输出的明确单位换算，未知单位必须报错，不能当成零。
func memorySize(text string) (int64, error) {
	for _, unit := range []struct {
		name  string
		scale float64
	}{{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"B", 1}} {
		if strings.HasSuffix(text, unit.name) {
			value, err := strconv.ParseFloat(strings.TrimSuffix(text, unit.name), 64)
			if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) || value*unit.scale >= math.MaxInt64 {
				return 0, fmt.Errorf("容器内存数值不合法")
			}
			return int64(value * unit.scale), nil
		}
	}
	return 0, fmt.Errorf("容器内存单位无法识别")
}
