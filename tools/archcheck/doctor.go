// 本文件：检查医生容器能从宿主机拿到什么。
//
// Go 导入检查只能管源码。即使代码没有引用场景包，把整个仓库挂进容器仍然会泄露答案。
// 因此部署配置也要检查，只允许那一个只读接入目录。
//
// 主要内容：
//   - doctorMountProblems：读取 Compose，检查医生的挂载和提权入口。
// 阅读建议：先看允许的唯一挂载，再看为什么拒绝 env_file 和 Docker 连接变量。

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
	if !ok || len(mounts) != 1 || mounts[0] != "./doctor/targets:/etc/doctor/targets:ro" {
		reject("只能挂载只读接入目录；不能读取仓库、场景、恢复记录或 Docker socket")
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
	return problems, nil
}
