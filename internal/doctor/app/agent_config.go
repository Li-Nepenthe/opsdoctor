// 本文件：读取模型、调度和页面安全配置，默认只运行不花钱的 Mock。
//
// 填了 Key 不等于授权自动使用它。只有明确把 LLM_PROVIDER 设成 openai，启动时
// 才要求真实地址、模型和单价。本机验收结束后恢复 mock，普通测试不会意外付费。
//
// 主要内容：agentSettings 保存私有配置；loadAgentConfig 校验范围；knownSecrets 收集脱敏值。
// 阅读建议：费用按整数微元计算，日志只能输出 provider、model 和数值预算。

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
	runbooks        string // 通用手册的只读目录，不是场景目录。
	apiFormat       string // Responses 与 Chat Completions 只在传输适配器里分流。
	currency        string // CNY 或 USD，账户和页面都显式带单位。
	diagnosisMicros int64  // 一次诊断上限，和当前账户使用相同币种。

	leaseTTL     time.Duration // 值班证有效期，续约间隔为三分之一。
	resumeMaxAge time.Duration // 中断超过这个期限，旧证据不再继续使用。

	provider      string        // mock 或兼容 OpenAI 协议的 openai。
	model         string        // mock 固定为脚本名；真实值来自部署配置。
	baseURL       string        // 只属于模型客户端，不是工具参数。
	key           string        // 仅在 openai 模式读取。
	timeout       time.Duration // 一轮模型请求连同重试的上限。
	prices        llm.Prices    // 缓存未命中输入价与输出价。
	budgetAccount string        // 跨重启保留的预算账户，阶段 7、8 共用。
	totalMicros   int64         // 进程重启不清零的全局累计上限。
	pendingAge    time.Duration // 告警排队超过此值不再执行。
	faultSilence  time.Duration // 整组恢复通知丢失时的兜底。
	concurrent    int           // 全局名额 1–2。
	perTarget     int           // 阶段 6 固定为 1。
	secureCookie  bool          // HTTPS 部署打开，回环 HTTP 演示关闭。
}

// loadAgentConfig 的空值是配置错误，不把误拼的真实模型模式静默退回 Mock。
func loadAgentConfig(reader config.Reader) (agentSettings, error) {
	var value agentSettings
	var problems []error
	read := func(name, fallback string) string {
		text, err := reader.String(name, fallback)
		problems = append(problems, err)
		return text
	}
	duration := func(name string, fallback, min, max time.Duration) time.Duration {
		v, err := reader.Duration(name, fallback, min, max)
		problems = append(problems, err)
		return v
	}
	integer := func(name string, fallback, min, max int) int {
		v, err := reader.Int(name, fallback, min, max)
		problems = append(problems, err)
		return v
	}
	money := func(name, fallback string) int64 {
		v, err := llm.Money(read(name, fallback))
		if err != nil || v <= 0 {
			problems = append(problems, fmt.Errorf("%s 必须是大于零、最多六位小数的金额", name))
		}
		return v
	}
	value.provider = read("LLM_PROVIDER", "mock")
	value.runbooks = read("DOCTOR_RUNBOOKS_DIR", "docs/runbooks")
	value.model = mock.Model
	value.currency = read("LLM_BUDGET_CURRENCY", "CNY")
	if value.currency != "CNY" && value.currency != "USD" {
		problems = append(problems, fmt.Errorf("LLM_BUDGET_CURRENCY 只支持 CNY 或 USD"))
	}
	legacy := read("LLM_TOTAL_BUDGET_CNY", "10")
	value.totalMicros = money("LLM_TOTAL_BUDGET", legacy)
	value.diagnosisMicros = money("LLM_DIAGNOSIS_BUDGET", "0.30")
	if value.diagnosisMicros > value.totalMicros {
		problems = append(problems, fmt.Errorf("单次诊断费用不能超过总预算"))
	}
	value.apiFormat = read("LLM_API_FORMAT", "chat_completions")
	if value.apiFormat != "chat_completions" && value.apiFormat != "responses" {
		problems = append(problems, fmt.Errorf("LLM_API_FORMAT 只支持 chat_completions 或 responses"))
	}
	value.budgetAccount = read("LLM_BUDGET_ACCOUNT", "phase-07-08")
	if len(value.budgetAccount) > 64 {
		problems = append(problems, fmt.Errorf("LLM_BUDGET_ACCOUNT 最多 64 字节"))
	}
	value.leaseTTL = duration("DOCTOR_LEASE_TTL", 30*time.Second, 3*time.Second, time.Minute)
	value.resumeMaxAge = duration("DOCTOR_RESUME_MAX_AGE", 2*time.Minute, time.Minute, 30*time.Minute)
	if value.resumeMaxAge < value.leaseTTL {
		problems = append(problems, fmt.Errorf("DOCTOR_RESUME_MAX_AGE 不能小于租约期限"))
	}
	value.timeout = duration("LLM_TIMEOUT", time.Minute, time.Second, 2*time.Minute)
	value.pendingAge = duration("DOCTOR_ALERT_PENDING_MAX_AGE", 5*time.Minute, time.Second, time.Hour)
	value.faultSilence = duration("DOCTOR_ALERT_SILENCE_TIMEOUT", 15*time.Minute, 3*time.Minute, 24*time.Hour)
	value.concurrent = integer("DOCTOR_MAX_CONCURRENT", 2, 1, 2)
	value.perTarget = integer("DOCTOR_MAX_CONCURRENT_PER_TARGET", 1, 1, 1)
	secure, err := reader.Bool("DOCTOR_COOKIE_SECURE", false)
	value.secureCookie = secure
	problems = append(problems, err)
	// mock 不读取 Key，更不会为了“检查连接”偷偷发一条真实请求。
	switch value.provider {
	case "mock":
	case "openai":
		value.baseURL = read("LLM_BASE_URL", "")
		value.model = read("LLM_MODEL", "")
		value.key = read("LLM_API_KEY", "")
		value.prices = llm.Prices{InputMicros: money("LLM_INPUT_PRICE_PER_MTOKEN", ""), CacheHitMicros: money("LLM_CACHE_HIT_PRICE_PER_MTOKEN", ""), OutputMicros: money("LLM_OUTPUT_PRICE_PER_MTOKEN", "")}
		if value.apiFormat == "responses" {
			value.prices.CacheWriteMicros = money("LLM_CACHE_WRITE_PRICE_PER_MTOKEN", "")
		}
		if strings.HasPrefix(value.model, "mock") || len(value.model) > 128 {
			problems = append(problems, fmt.Errorf("真实模型名不能以 mock 开头，且最多 128 字节"))
		}
	default:
		problems = append(problems, fmt.Errorf("LLM_PROVIDER 只接受 mock 或 openai"))
	}
	return value, errors.Join(problems...)
}

// knownSecrets 只收集本项目明确使用的凭据，在内存中替换，不输出也不另存清单。
func knownSecrets() []string {
	var values []string
	for _, name := range []string{"JWT_SECRET", "DOCTOR_ADMIN_PASSWORD", "DOCTOR_MYSQL_PASSWORD", "DOCTOR_MYSQL_ROOT_PASSWORD", "SHOP_ALERT_TOKEN", "SHOP_MYSQL_RO_PASSWORD", "SHOP_REDIS_RO_PASSWORD", "MYSQL_PASSWORD", "MYSQL_ROOT_PASSWORD", "REDIS_PASSWORD"} {
		if value := os.Getenv(name); value != "" {
			values = append(values, value)
		}
	}
	// 容器只拿到连接串时，独立的 PASSWORD 变量可能不存在。解析时只取密码，
	// 不把整条连接串留下来，避免报错或模型恰好提到它时绕过精确替换。
	for _, entry := range os.Environ() {
		name, value, _ := strings.Cut(entry, "=")
		// 新接入系统的运维和 MCP 令牌也要脱敏，不仅限于 shop 的已知变量名。
		if (strings.HasSuffix(name, "_TOKEN") || strings.HasSuffix(name, "_PASSWORD") || strings.HasSuffix(name, "_API_KEY")) && value != "" {
			values = append(values, value)
		}
		if strings.HasSuffix(name, "_DSN") {
			if parsed, err := mysql.ParseDSN(value); err == nil && parsed.Passwd != "" {
				values = append(values, parsed.Passwd)
			}
		}
		if strings.HasSuffix(name, "_URL") {
			if parsed, err := url.Parse(value); err == nil && parsed.User != nil {
				if password, ok := parsed.User.Password(); ok {
					values = append(values, password)
				}
			}
		}
	}
	return values
}
