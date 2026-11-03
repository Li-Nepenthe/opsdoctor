// 本文件：为外部 Demo 单独测量 Docker 实际内存，避免套用 shop 的六 GiB 暂停线。
//
// 两套靶场轮流运行，医生仍然要在线。这里统计当前全部运行容器的内存，并给
// Docker 虚拟机保留两 GiB 余量；达到边界就停止实验，不改系统资源设置。
//
// 主要内容：diagnosisMemory 选择当前实验的测量方法；demoMemoryBytes 读取真实占用。
// 阅读建议：context 中的函数只影响本次外部实验，旧 shop 命令仍走原测量函数。

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// diagnosisMemoryKey 只在宿主机当前评测上下文传递，不写入诊断参数或开放给模型。
type diagnosisMemoryKey struct{}

// demoClockKey 让第二系统采样专用医生容器；原医生和阶段 8 的命令保持原样。
type demoClockKey struct{}

// clockSampleArguments 只选择固定容器，不接受任意 shell 或模型传入的命令。
func clockSampleArguments(ctx context.Context) []string {
	if ctx.Value(demoClockKey{}) != nil {
		return []string{"exec", "opsdoctor-phase09-doctor", "/app/server", "--clock-samples"}
	}
	return []string{"compose", "--env-file", ".env", "-f", "deploy/docker-compose.yml", "exec", "-T", "doctor", "/app/server", "--clock-samples"}
}

// checkDemoIsolation 同时拒绝两套靶场和遗留测试项目运行，内存足够也不能绕过这条纪律。
func checkDemoIsolation(labels string) error {
	for _, label := range strings.Fields(labels) {
		project, service, _ := strings.Cut(label, "/")
		if !strings.HasPrefix(project, "opsdoctor") || project == "opsdoctor-otel-demo" {
			continue
		}
		if project == "opsdoctor" && service == "doctor-mysql" || project == "opsdoctor-phase09-doctor" && service == "doctor" {
			continue
		}
		return fmt.Errorf("发现另一套靶场或遗留测试容器仍运行：%s；只允许 Demo、阶段 9 专用医生和原 doctor-mysql", label)
	}
	return nil
}

// diagnosisMemory 让相同的诊断等待器使用所属靶场的内存检查。
func diagnosisMemory(ctx context.Context) (int64, error) {
	if measure, ok := ctx.Value(diagnosisMemoryKey{}).(func(context.Context) (int64, error)); ok {
		return measure(ctx)
	}
	return memoryBytes(ctx)
}

// demoMemoryBytes 读取 Docker 的真实上限与当前全部容器占用，不靠上限之和估算。
func demoMemoryBytes(parent context.Context) (int64, error) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	// 先检查运行身份，再测内存，不能因为机器够大就同时运行两套靶场。
	labels, err := exec.CommandContext(ctx, "docker", "ps", "--format", `{{.Label "com.docker.compose.project"}}/{{.Label "com.docker.compose.service"}}`).Output()
	if err != nil {
		return 0, fmt.Errorf("无法检查两套靶场隔离状态")
	}
	if err = checkDemoIsolation(string(labels)); err != nil {
		return 0, err
	}
	limitText, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.MemTotal}}").Output()
	if err != nil {
		return 0, fmt.Errorf("无法读取 Docker 内存上限")
	}
	limit, err := strconv.ParseInt(strings.TrimSpace(string(limitText)), 10, 64)
	if err != nil || limit <= 2<<30 {
		return 0, fmt.Errorf("当前 Docker 内存上限无效或不足以保留两 GiB 余量")
	}
	ids, err := exec.CommandContext(ctx, "docker", "ps", "-q").Output()
	if err != nil || len(strings.Fields(string(ids))) == 0 {
		return 0, fmt.Errorf("当前 Docker 没有可测量的运行容器")
	}
	args := append([]string{"stats", "--no-stream", "--format", "{{json .}}"}, strings.Fields(string(ids))...)
	raw, err := exec.CommandContext(ctx, "docker", args...).Output()
	if err != nil {
		return 0, fmt.Errorf("外部靶场 Demo 容器内存测量未完成")
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != len(strings.Fields(string(ids))) {
		return 0, fmt.Errorf("当前 Docker 内存样本不完整")
	}
	var total int64
	// 少一份内存样本就报错，避免把未测到的容器当成零占用。
	for _, line := range lines {
		var sample struct {
			Memory string `json:"MemUsage"`
		}
		if json.Unmarshal([]byte(line), &sample) != nil {
			return 0, fmt.Errorf("当前 Docker 内存响应格式不符")
		}
		size, err := memorySize(strings.TrimSpace(strings.SplitN(sample.Memory, "/", 2)[0]))
		if err != nil {
			return 0, err
		}
		total += size
	}
	if total > limit-(2<<30) {
		return total, fmt.Errorf("当前 Docker 实际容器内存 %.3f GiB，上限 %.3f GiB，已不足两 GiB 余量；停止并由用户决定资源设置", float64(total)/(1<<30), float64(limit)/(1<<30))
	}
	return total, nil
}
