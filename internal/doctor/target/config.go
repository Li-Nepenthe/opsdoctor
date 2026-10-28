// 本文件：读取一个目录里的全部系统配置，并检查系统之间有没有共用凭据。
//
// 配置属于部署者，工具参数属于调用者。两者分开后，工具运行时就不能临时换主机。
// 密码只从配置指定的环境变量读取，错误信息只写变量名，不回显内容。
//
// 主要内容：
//   - Config：本阶段允许接入的数据源字段。
//   - readConfigs、readFile：读取有大小上限的普通文件，拒绝链接和子目录路径。
//   - validateConfigs：检查重名、账号变量共用以及系统前缀。
//
// 阅读建议：先看 Config，再看两个系统的配置如何一起校验。

package target

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// Config 只保存地址和变量名；阶段 7 的写入口尚未开放，也不能提前填进来。
type Config struct {
	AlertRecoveryWindow string            `yaml:"alert_recovery_window"`  // 整组恢复后继续接收迟到症状的时间，由各系统的告警规则决定。
	Name                string            `yaml:"name"`                   // 在目录中唯一，之后成为任务绑定的系统名。
	Profile             string            `yaml:"profile"`                // 同目录的架构说明文件名。
	AlertTokenEnv       string            `yaml:"alert_token_env"`        // 本系统的推送令牌变量。
	Prometheus          string            `yaml:"prometheus"`             // 指标后端根地址。
	Jaeger              string            `yaml:"jaeger"`                 // 调用链查询根地址。
	Loki                string            `yaml:"loki"`                   // 日志查询根地址。
	MySQLDSNEnv         string            `yaml:"mysql_readonly_dsn_env"` // 只读 MySQL 连接配置的变量名。
	RedisURLEnv         string            `yaml:"redis_readonly_url_env"` // 只读 Redis 连接配置的变量名。
	Pprof               map[string]string `yaml:"pprof"`                  // 服务名到堆栈端点的固定映射。
}

// readConfigs 按文件名顺序读取全部 YAML，目录为空也报错，避免“启动成功但什么都接不上”。
func readConfigs(directory string) ([]Config, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("无法读取 TARGETS_DIR")
	}
	configs := []Config{}
	for _, entry := range entries {
		extension := filepath.Ext(entry.Name())
		if extension != ".yaml" && extension != ".yml" {
			continue
		}
		data, err := readFile(directory, entry.Name(), 64<<10)
		if err != nil {
			return nil, err
		}
		var config Config
		// Strict 同时拒绝未知字段与重复键。拼错字段不能静默丢掉某个数据源。
		if err := yaml.UnmarshalWithOptions(data, &config, yaml.Strict()); err != nil {
			return nil, fmt.Errorf("接入配置 %s 格式错误或含未知字段", entry.Name())
		}
		configs = append(configs, config)
	}
	if len(configs) == 0 || len(configs) > 32 {
		return nil, fmt.Errorf("TARGETS_DIR 必须包含 1–32 个系统配置")
	}
	return configs, validateConfigs(configs)
}

// readFile 只接受目录里的普通文件，配置不能通过 ../ 或符号链接读到实验答案。
func readFile(directory, name string, limit int64) ([]byte, error) {
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, "/\\:") {
		return nil, fmt.Errorf("接入文件必须位于 TARGETS_DIR 中")
	}
	path := filepath.Join(directory, name)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("接入文件 %s 不存在、不是普通文件或超过大小上限", name)
	}
	data, err := os.ReadFile(path)
	if err != nil || int64(len(data)) > limit {
		return nil, fmt.Errorf("无法读取接入文件 %s", name)
	}
	return data, nil
}

// validateConfigs 先查跨系统冲突，再查前缀，错误能直接说明两个系统共用了哪个变量。
func validateConfigs(configs []Config) error {
	// 先查重，再查命名规则。这样报错可以直接指出哪两个系统共用了同一个变量。
	names, variables := map[string]bool{}, map[string]string{}
	namePattern := regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	for _, config := range configs {
		if !namePattern.MatchString(config.Name) {
			return fmt.Errorf("系统名必须是小写字母开头，且只含字母、数字或短横线")
		}
		if names[config.Name] {
			return fmt.Errorf("系统名 %s 重复", config.Name)
		}
		names[config.Name] = true
		for _, variable := range config.variables() {
			if variable == "" {
				continue
			}
			if previous, exists := variables[variable]; exists {
				return fmt.Errorf("系统 %s 与 %s 共用凭据变量 %s", previous, config.Name, variable)
			}
			variables[variable] = config.Name
		}
	}
	for _, config := range configs {
		prefix := strings.ToUpper(strings.ReplaceAll(config.Name, "-", "_")) + "_"
		for _, variable := range config.variables() {
			if variable != "" && (!strings.HasPrefix(variable, prefix) || !regexp.MustCompile(`^[A-Z][A-Z0-9_]+$`).MatchString(variable)) {
				return fmt.Errorf("系统 %s 的凭据变量必须以 %s 开头", config.Name, prefix)
			}
		}
		if config.AlertTokenEnv == "" || !strings.HasSuffix(config.Profile, ".profile.md") {
			return fmt.Errorf("系统 %s 缺少告警令牌变量或 .profile.md 说明文件", config.Name)
		}
		// 不能用全局默认掩盖缺项：每个系统的告警持续时间可能不同。
		window, err := time.ParseDuration(config.AlertRecoveryWindow)
		if err != nil || window <= 0 || window > 24*time.Hour {
			return fmt.Errorf("系统 %s 的 alert_recovery_window 必须是大于 0、且不超过 24h 的时长", config.Name)
		}
		for service := range config.Pprof {
			if !namePattern.MatchString(service) {
				return fmt.Errorf("系统 %s 的 pprof 服务名不合法", config.Name)
			}
		}
	}
	return nil
}

// variables 只包含本阶段真正使用的凭据，不提前向医生传递写账号。
func (config Config) variables() []string {
	return []string{config.AlertTokenEnv, config.MySQLDSNEnv, config.RedisURLEnv}
}
