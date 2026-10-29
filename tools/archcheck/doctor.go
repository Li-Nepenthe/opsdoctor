// 本文件：检查医生容器能从宿主机拿到什么。
//
// Go 导入检查只能管源码。即使代码没有引用场景包，把整个仓库挂进容器仍然会泄露答案。
// 因此部署配置也要检查，只允许只读接入目录和阶段 8 的通用手册目录。
//
// 主要内容：
//   - doctorMountProblems：读取 Compose，检查医生的挂载和提权入口。
//   - opsPortProblems：确保业务运维端口只在容器网络内使用。
// 阅读建议：先看两个精确允许的挂载，再看为什么拒绝 env_file 和 Docker 连接变量。

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
)

// doctorMountProblems 使用完整 YAML 解析，长格式挂载也不能绕过只接受固定字符串的规则。
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
		return nil, nil
	} // 旧阶段还没有医生服务，无需强行补一个。
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
	// 这些入口可以绕过普通文件挂载限制；本阶段的医生没有使用它们的理由。
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

// opsPortProblems 同时识别 Compose 两种端口格式，不能只靠“没有写 localhost”猜是否公开。
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
				// 短格式最后一段是容器端口，前面的 IP 和宿主端口不改变判断。
				parts := strings.Split(strings.Split(value, "/")[0], ":")
				target = parts[len(parts)-1]
			case map[string]any:
				// 长格式显式写 target；published 使用其他数字也不能放行。
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
