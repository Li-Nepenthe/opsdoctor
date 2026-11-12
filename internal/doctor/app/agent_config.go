// 本文件：模型和调度配置在服务接收请求前校验。
// loadAgentConfig 默认选用 Mock，只有显式选择真实提供方时才读取其凭据和单价。

package app

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm/mock"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/go-sql-driver/mysql"
)

// agentSettings 含 API Key，不能作为整个对象写进日志或错误。
type agentSettings struct {
	pricesAt        func(time.Time) llm.Prices // 配对 DeepSeek 按请求时间选择价格，主模型为空。
	comparison      *agentSettings             // 仅真实评测且显式开启时装配；普通 Mock 不读取其 Key。
	runbooks        string                     // 通用手册的只读目录，不是场景目录。
	apiFormat       string                     // Responses 与 Chat Completions 只在传输适配器里分流。
	currency        string                     // CNY 或 USD，账户和页面都显式带单位。
	diagnosisMicros int64                      // 一次诊断上限，和当前账户使用相同币种。

	leaseTTL     time.Duration // 续约间隔为租约时长的三分之一。
	resumeMaxAge time.Duration // 中断超过这个期限，旧证据不再继续使用。

	provider      string
	model         string        // mock 固定为脚本名；真实值来自部署配置。
	baseURL       string        // 只属于模型客户端，不是工具参数。
	key           string        // 仅在 openai 模式读取。
	timeout       time.Duration // 一轮模型请求连同重试的上限。
	prices        llm.Prices
	budgetAccount string        // 账户跨重启持续累计。
	totalMicros   int64         // 进程重启不清零的全局累计上限。
	pendingAge    time.Duration // 告警排队超过此值不再执行。
	faultSilence  time.Duration // 整组恢复通知丢失时的兜底。
	concurrent    int
	perTarget     int  // 每个系统同时只运行一项诊断。
	secureCookie  bool // HTTPS 部署打开，回环 HTTP 演示关闭。
}

// loadAgentConfig 的空值是配置错误，不把误拼的真实模型模式静默退回 Mock。
func loadAgentConfig(reader config.Reader) (agentSettings, error) {
	inputs := settingsReader{source: reader, amountError: "%s 必须是大于零、最多六位小数的金额"}
	var value agentSettings
	inputs.readModelPolicy(&value)
	inputs.readScheduling(&value)
	inputs.readProviderCredentials(&value)
	if err := errors.Join(inputs.problems...); err != nil {
		return value, err
	}
	comparison, err := loadComparison(reader, value)
	value.comparison = comparison
	return value, err
}

// 保留读取顺序及每项校验错误，启动失败时一次报告所有配置问题。
type settingsReader struct {
	source      config.Reader
	problems    []error
	amountError string
}

func (inputs *settingsReader) read(name, fallback string) string {
	text, err := inputs.source.String(name, fallback)
	inputs.problems = append(inputs.problems, err)
	return text
}

func (inputs *settingsReader) duration(name string, fallback, min, max time.Duration) time.Duration {
	value, err := inputs.source.Duration(name, fallback, min, max)
	inputs.problems = append(inputs.problems, err)
	return value
}

func (inputs *settingsReader) integer(name string, fallback, min, max int) int {
	value, err := inputs.source.Int(name, fallback, min, max)
	inputs.problems = append(inputs.problems, err)
	return value
}

func (inputs *settingsReader) money(name, fallback string) int64 {
	value, err := llm.Money(inputs.read(name, fallback))
	if err != nil || value <= 0 {
		inputs.problems = append(inputs.problems, fmt.Errorf(inputs.amountError, name))
	}
	return value
}

func (inputs *settingsReader) readModelPolicy(value *agentSettings) {
	value.provider = inputs.read("LLM_PROVIDER", "mock")
	value.runbooks = inputs.read("DOCTOR_RUNBOOKS_DIR", "docs/runbooks")
	value.model = mock.Model
	value.currency = inputs.read("LLM_BUDGET_CURRENCY", "CNY")
	if value.currency != "CNY" && value.currency != "USD" {
		inputs.problems = append(inputs.problems, fmt.Errorf("LLM_BUDGET_CURRENCY 只支持 CNY 或 USD"))
	}
	legacy := inputs.read("LLM_TOTAL_BUDGET_CNY", "10")
	value.totalMicros = inputs.money("LLM_TOTAL_BUDGET", legacy)
	value.diagnosisMicros = inputs.money("LLM_DIAGNOSIS_BUDGET", "0.30")
	if value.diagnosisMicros > value.totalMicros {
		inputs.problems = append(inputs.problems, fmt.Errorf("单次诊断费用不能超过总预算"))
	}
	value.apiFormat = inputs.read("LLM_API_FORMAT", "chat_completions")
	if value.apiFormat != "chat_completions" && value.apiFormat != "responses" {
		inputs.problems = append(inputs.problems, fmt.Errorf("LLM_API_FORMAT 只支持 chat_completions 或 responses"))
	}
	value.budgetAccount = inputs.read("LLM_BUDGET_ACCOUNT", "phase-07-08")
	if len(value.budgetAccount) > 64 {
		inputs.problems = append(inputs.problems, fmt.Errorf("LLM_BUDGET_ACCOUNT 最多 64 字节"))
	}
}

func (inputs *settingsReader) readScheduling(value *agentSettings) {
	value.leaseTTL = inputs.duration("DOCTOR_LEASE_TTL", 30*time.Second, 3*time.Second, time.Minute)
	value.resumeMaxAge = inputs.duration("DOCTOR_RESUME_MAX_AGE", 2*time.Minute, time.Minute, 30*time.Minute)
	if value.resumeMaxAge < value.leaseTTL {
		inputs.problems = append(inputs.problems, fmt.Errorf("DOCTOR_RESUME_MAX_AGE 不能小于租约期限"))
	}
	value.timeout = inputs.duration("LLM_TIMEOUT", time.Minute, time.Second, 2*time.Minute)
	value.pendingAge = inputs.duration("DOCTOR_ALERT_PENDING_MAX_AGE", 5*time.Minute, time.Second, time.Hour)
	value.faultSilence = inputs.duration("DOCTOR_ALERT_SILENCE_TIMEOUT", 15*time.Minute, 3*time.Minute, 24*time.Hour)
	value.concurrent = inputs.integer("DOCTOR_MAX_CONCURRENT", 2, 1, 2)
	value.perTarget = inputs.integer("DOCTOR_MAX_CONCURRENT_PER_TARGET", 1, 1, 1)
	secure, err := inputs.source.Bool("DOCTOR_COOKIE_SECURE", false)
	value.secureCookie = secure
	inputs.problems = append(inputs.problems, err)
}

func (inputs *settingsReader) readProviderCredentials(value *agentSettings) {
	// Mock 不读取模型密钥，也不进行连通性探测。
	switch value.provider {
	case "mock":
	case "openai":
		value.baseURL = inputs.read("LLM_BASE_URL", "")
		value.model = inputs.read("LLM_MODEL", "")
		value.key = inputs.read("LLM_API_KEY", "")
		value.prices = llm.Prices{
			InputMicros:    inputs.money("LLM_INPUT_PRICE_PER_MTOKEN", ""),
			CacheHitMicros: inputs.money("LLM_CACHE_HIT_PRICE_PER_MTOKEN", ""),
			OutputMicros:   inputs.money("LLM_OUTPUT_PRICE_PER_MTOKEN", ""),
		}
		if value.apiFormat == "responses" {
			value.prices.CacheWriteMicros = inputs.money("LLM_CACHE_WRITE_PRICE_PER_MTOKEN", "")
		}
		if strings.HasPrefix(value.model, "mock") || len(value.model) > 128 {
			inputs.problems = append(inputs.problems, fmt.Errorf("真实模型名不能以 mock 开头，且最多 128 字节"))
		}
	default:
		inputs.problems = append(inputs.problems, fmt.Errorf("LLM_PROVIDER 只接受 mock 或 openai"))
	}
}

// knownSecrets 只收集本项目明确使用的凭据，在内存中替换，不输出也不另存清单。
func knownSecrets() []string {
	var values []string
	for _, name := range []string{"JWT_SECRET", "DOCTOR_ADMIN_PASSWORD", "DOCTOR_MYSQL_PASSWORD", "DOCTOR_MYSQL_ROOT_PASSWORD", "SHOP_ALERT_TOKEN", "SHOP_MYSQL_RO_PASSWORD", "SHOP_REDIS_RO_PASSWORD", "MYSQL_PASSWORD", "MYSQL_ROOT_PASSWORD", "REDIS_PASSWORD"} {
		if value := os.Getenv(name); value != "" {
			values = append(values, value)
		}
	}
	// 容器可能只有连接串。只提取其中的密码，让脱敏器也能匹配单独出现的密码。
	for _, entry := range os.Environ() {
		name, value, _ := strings.Cut(entry, "=")
		if secret, ok := environmentSecret(name, value); ok {
			values = append(values, secret)
		}
	}
	return values
}

// 按各系统约定的后缀识别凭据，连接串只提取其中的密码。
func environmentSecret(name, value string) (string, bool) {
	switch {
	case strings.HasSuffix(name, "_TOKEN"), strings.HasSuffix(name, "_PASSWORD"), strings.HasSuffix(name, "_API_KEY"):
		return value, value != ""
	case strings.HasSuffix(name, "_DSN"):
		if parsed, err := mysql.ParseDSN(value); err == nil && parsed.Passwd != "" {
			return parsed.Passwd, true
		}
	case strings.HasSuffix(name, "_URL"):
		if parsed, err := url.Parse(value); err == nil && parsed.User != nil {
			return parsed.User.Password()
		}
	}
	return "", false
}
