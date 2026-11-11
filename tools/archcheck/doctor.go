// 本文件：检查 Compose 是否给医生开放了额外权限，以及业务运维端口是否对宿主机开放。
// 从 doctorMountProblems 读起，医生只能挂载只读接入目录和通用手册。

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
)

func doctorMountProblems(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var compose struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil, err
	}
	doctor, exists := compose.Services["doctor"]
	if !exists {
		// 早期阶段的 Compose 尚未部署医生。
		return nil, nil
	}
	var problems []string
	reject := func(reason string) { problems = append(problems, fmt.Sprintf("%s:1: doctor %s", path, reason)) }
	mounts, ok := doctor["volumes"].([]any)
	allowed := map[string]bool{"./doctor/targets:/etc/doctor/targets:ro": true, "../docs/runbooks:/etc/doctor/runbooks:ro": true}
	seen := map[string]bool{}
	if !ok || len(mounts) < 1 || len(mounts) > 2 {
		reject("只能挂载只读接入目录和通用手册")
	}
	for _, mount := range mounts {
		name, valid := mount.(string)
		if !valid || !allowed[name] || seen[name] {
			reject("挂载必须精确匹配只读接入目录或通用手册；不能读取仓库、场景、恢复记录或 Docker socket")
		}
		seen[name] = true
	}
	if !seen["./doctor/targets:/etc/doctor/targets:ro"] {
		reject("缺少只读接入目录")
	}
	// 只核对挂载还不够，额外的容器权限或整份环境文件也能让医生接触宿主机资源。
	for _, key := range []string{"volumes_from", "devices", "privileged", "cap_add", "pid", "network_mode", "env_file"} {
		if _, exists := doctor[key]; exists {
			reject("不得配置 " + key)
		}
	}
	environment, ok := doctor["environment"].(map[string]any)
	if !ok {
		reject("环境变量必须逐项列出，不能整份传入宿主环境")
	}
	for key := range environment {
		if strings.HasPrefix(key, "CHAOS_") || strings.HasPrefix(key, "DOCKER_") || key == "MYSQL_DSN" || key == "REDIS_PASSWORD" {
			reject("不得接收实验控制或靶场业务凭据变量 " + key)
		}
	}
	return append(problems, opsPortProblems(path, compose.Services)...), nil
}

func opsPortProblems(path string, services map[string]map[string]any) []string {
	var problems []string
	for _, name := range []string{"inventory-service", "order-service", "order-worker"} {
		service := services[name]
		// 主机网络不需要 ports 就能公开监听端口，因此先单独拒绝它。
		if service["network_mode"] == "host" {
			problems = append(problems, fmt.Sprintf("%s:1: %s 运维接口不能使用主机网络", path, name))
		}
		ports, _ := service["ports"].([]any)
		for _, port := range ports {
			target := ""
			switch value := port.(type) {
			case string:
				// 即使只发布到本机回环地址，运维端口也已经离开容器网络。
				parts := strings.Split(strings.Split(value, "/")[0], ":")
				target = parts[len(parts)-1]
			case map[string]any:
				// 判断的是容器端口，宿主机映射到别的端口号也要报错。
				target = fmt.Sprint(value["target"])
			default:
				target = fmt.Sprint(value)
			}
			if target == "6064" {
				problems = append(problems, fmt.Sprintf("%s:1: %s 的 6064 运维端口只能在内部网络开放", path, name))
			}
		}
	}
	return problems
}
