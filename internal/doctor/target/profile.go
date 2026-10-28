// 本文件：从架构说明中读取允许用于报告的组件名，并列出缺失的数据源。
//
// 组件名不能由模型临时发明，否则同一个服务的不同拼法无法比较。说明书既给人看，
// 也提供唯一命名表；医生只读这张表，不读取任何实验答案。
//
// 主要内容：componentNames 解析命名表；Profile、Components、Unavailable 提供诊断上下文。
// 阅读建议：只有两个标记之间的表格第一列会被当成标识，普通正文不会变成允许项。

package target

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// componentNames 拒绝空表、重复项和过长标识，错误配置在第一次诊断之前暴露。
func componentNames(profile string) ([]string, error) {
	_, rest, ok := strings.Cut(profile, "<!-- COMPONENTS_START -->")
	if !ok {
		return nil, fmt.Errorf("说明书缺少 COMPONENTS_START 组件命名表")
	}
	block, _, ok := strings.Cut(rest, "<!-- COMPONENTS_END -->")
	if !ok {
		return nil, fmt.Errorf("说明书缺少 COMPONENTS_END")
	}
	names := []string{}
	pattern := regexp.MustCompile(`^[a-z][a-z0-9_.:>\-]{0,127}$`)
	for _, line := range strings.Split(block, "\n") {
		columns := strings.Split(strings.TrimSpace(line), "|")
		if len(columns) < 3 {
			continue
		}
		name := strings.TrimSpace(columns[1])
		// 标题和分隔行没有反引号；只有明确写成代码的第一列才是机器标识。
		if !strings.HasPrefix(name, "`") || !strings.HasSuffix(name, "`") {
			continue
		}
		name = strings.Trim(name, "`")
		if !pattern.MatchString(name) || slices.Contains(names, name) {
			return nil, fmt.Errorf("组件标识非法或重复")
		}
		names = append(names, name)
	}
	if len(names) < 2 || len(names) > 128 || !slices.Contains(names, "unknown") {
		return nil, fmt.Errorf("组件表需要 unknown 和至少一个架构组件，最多 128 项")
	}
	return names, nil
}

// Profile 返回部署者提供的架构文字；发送给模型之前还要经过统一脱敏。
func (system *System) Profile() string { return system.profile }

// Components 返回副本，调用者不能在一次诊断中扩大允许的组件集合。
func (system *System) Components() []string { return slices.Clone(system.components) }

// Unavailable 明确告诉模型缺哪些证据，不能把“没有配置”解释成“没有故障”。
func (system *System) Unavailable() []string { return slices.Clone(system.unavailable) }

// missingSources 用配置是否存在判断可用能力；运行中的查询失败仍由工具单独报告。
func missingSources(config Config) []string {
	missing := []string{}
	for _, item := range []struct {
		name    string
		present bool
	}{
		{"指标", config.Prometheus != ""}, {"调用链", config.Jaeger != ""}, {"日志", config.Loki != ""},
		{"MySQL 检查", config.MySQLDSNEnv != ""}, {"Redis 检查", config.RedisURLEnv != ""}, {"goroutine 堆栈", len(config.Pprof) > 0},
	} {
		if !item.present {
			missing = append(missing, item.name)
		}
	}
	return missing
}
