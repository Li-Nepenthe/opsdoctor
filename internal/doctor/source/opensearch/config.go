// 本文件：校验部署者指定的 OpenSearch 根地址、单一索引范围和日志字段。调用工具时不能扩大索引范围。

package opensearch

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// maximumIndexPrefixBytes 限制部署配置的索引前缀长度，末尾的一个通配符不计入前缀。
const maximumIndexPrefixBytes = 128

// maximumFieldBytes 给嵌套属性路径留出空间，同时限制递归读取字段的最大深度。
const maximumFieldBytes = 256

// Fields 的值是后端字段路径，点号既可以分隔对象，也可以出现在原始属性键里。
type Fields struct {
	Service   string `yaml:"service" json:"service"`     // 精确匹配服务；text 字段应填写它的 keyword 子字段。
	Level     string `yaml:"level" json:"level"`         // 精确匹配日志级别，值沿用 DEBUG、INFO、WARN、ERROR。
	Message   string `yaml:"message" json:"message"`     // 普通文字消息，关键词使用 match_phrase。
	Timestamp string `yaml:"timestamp" json:"timestamp"` // OpenSearch 的 date/date_nanos 字段，读取后转成 Unix 纳秒。
	TraceID   string `yaml:"trace_id" json:"trace_id"`   // 精确匹配请求标识，空查询不添加这个条件。
}

// Config 只在创建客户端时使用；密码应由接入层从环境变量取得。
type Config struct {
	URL      string // 没有账号、路径和查询串的 HTTP(S) 根地址。
	Index    string // 单个索引或有明确前缀的通配符，例如 logs-otel-*。
	Fields   Fields // 五项映射都必须给出，避免查询静默退化。
	Username string // 安全插件开启时使用只有读取此索引权限的账号。
	Password string // 只从环境变量传入，不进入日志和工具结果。
}

// Validate 在发送请求以前拒绝不完整配置；校验错误只指出字段名，不回显凭据。
func (config Config) Validate() error {
	base, err := url.Parse(config.URL)
	if err != nil || base.Hostname() == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Path != "" && base.Path != "/") {
		return fmt.Errorf("opensearch.url 必须是没有账号和路径的 HTTP(S) 根地址")
	}
	// 不接受逗号、斜杠、_all 和孤立的 *，防止配置错误扩大成所有索引。
	if len(strings.TrimSuffix(config.Index, "*")) > maximumIndexPrefixBytes || !regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*\*?$`).MatchString(config.Index) {
		return fmt.Errorf("opensearch.index 必须是单个索引或带固定前缀的通配符")
	}
	fieldPattern := regexp.MustCompile(`^[@a-zA-Z_][@a-zA-Z0-9_.-]*$`)
	for name, field := range map[string]string{"service": config.Fields.Service, "level": config.Fields.Level, "message": config.Fields.Message, "timestamp": config.Fields.Timestamp, "trace_id": config.Fields.TraceID} {
		if len(field) > maximumFieldBytes || !fieldPattern.MatchString(field) || strings.Contains(field, "..") || strings.HasSuffix(field, ".") {
			return fmt.Errorf("opensearch.fields.%s 缺失或不是合法字段路径", name)
		}
	}
	if (config.Username == "") != (config.Password == "") {
		return fmt.Errorf("OpenSearch 只读用户名和密码必须成对提供")
	}
	return nil
}
