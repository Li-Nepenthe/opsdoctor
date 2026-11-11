// 本文件：固定医生启动配置及不同模型配置分支实际查找的环境变量，只执行加载函数。

package app

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestDoctorRedactionEnvironmentContract 已接入系统可以有不同的变量前缀，收集脱敏值时要按约定后缀找到它们。
func TestDoctorRedactionEnvironmentContract(t *testing.T) {
	fixtures := map[string]string{"FIXTURE_API_KEY": "fake-api-key-11", "FIXTURE_TOKEN": "fake-token-22", "FIXTURE_PASSWORD": "fake-password-33", "JWT_SECRET": "fake-jwt-44"}
	for name, value := range fixtures {
		t.Setenv(name, value)
	}
	t.Setenv("FIXTURE_DSN", "reader:fake-db-55@tcp(localhost:3306)/fixture")
	t.Setenv("FIXTURE_URL", "redis://reader:fake-url-66@localhost:6379/0")
	secrets := knownSecrets()
	for name, want := range fixtures {
		if !slices.Contains(secrets, want) {
			t.Fatalf("没有收集 %s", name)
		}
	}
	if !slices.Contains(secrets, "fake-db-55") || !slices.Contains(secrets, "fake-url-66") {
		t.Fatal("连接串中的密码没有单独收集")
	}
}

// TestDoctorStartupEnvironmentContract 医生自身的配置入口直接读取进程环境，给它一套独立的假账号，仅做本地解析。
func TestDoctorStartupEnvironmentContract(t *testing.T) {
	for _, key := range []string{"DOCTOR_ADDR", "TARGETS_DIR"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("DOCTOR_MYSQL_DSN", "doctor:fixture@tcp(localhost:3306)/doctor")
	t.Setenv("DOCTOR_ADMIN_EMAIL", "ops@example.invalid")
	t.Setenv("DOCTOR_ADMIN_PASSWORD", "fixture-passphrase")
	t.Setenv("JWT_SECRET", strings.Repeat("fixture-", 6))
	value, err := loadConfig()
	if err != nil || value.address != ":8090" || value.targets != "deploy/doctor/targets" || value.email != "ops@example.invalid" {
		t.Fatalf("医生启动配置：%v", err)
	}
	// 空串和未设置的含义不同。错误列出变量名，不包含传入的测试凭据。
	for _, key := range strings.Fields("DOCTOR_ADDR TARGETS_DIR DOCTOR_MYSQL_DSN DOCTOR_ADMIN_EMAIL DOCTOR_ADMIN_PASSWORD JWT_SECRET") {
		t.Setenv(key, "")
	}
	_, err = loadConfig()
	if err == nil {
		t.Fatal("缺少配置仍然启动")
	}
	for _, key := range strings.Fields("DOCTOR_ADDR TARGETS_DIR DOCTOR_MYSQL_DSN DOCTOR_ADMIN_EMAIL DOCTOR_ADMIN_PASSWORD JWT_SECRET") {
		if !strings.Contains(err.Error(), "环境变量 "+key+" 不能为空") {
			t.Fatalf("启动错误未指出 %s：%v", key, err)
		}
	}
}

// TestAgentEnvironmentContract 记录免费模式实际读取的变量，并核对启动时生效的默认值和校验结果。
func TestAgentEnvironmentContract(t *testing.T) {
	keys := map[string]bool{}
	read := func(override, raw string) config.Reader {
		return config.NewReader(func(name string) (string, bool) {
			keys[name] = true
			if name == override {
				return raw, true
			}
			return "", false
		})
	}
	settings, err := loadAgentConfig(read("", ""))
	if err != nil {
		t.Fatal(err)
	}
	defaults := map[string]any{
		"LLM_PROVIDER":                     settings.provider,
		"DOCTOR_RUNBOOKS_DIR":              settings.runbooks,
		"LLM_BUDGET_CURRENCY":              settings.currency,
		"LLM_TOTAL_BUDGET_CNY":             settings.totalMicros,
		"LLM_TOTAL_BUDGET":                 settings.totalMicros,
		"LLM_DIAGNOSIS_BUDGET":             settings.diagnosisMicros,
		"LLM_API_FORMAT":                   settings.apiFormat,
		"LLM_BUDGET_ACCOUNT":               settings.budgetAccount,
		"DOCTOR_LEASE_TTL":                 settings.leaseTTL,
		"DOCTOR_RESUME_MAX_AGE":            settings.resumeMaxAge,
		"LLM_TIMEOUT":                      settings.timeout,
		"DOCTOR_ALERT_PENDING_MAX_AGE":     settings.pendingAge,
		"DOCTOR_ALERT_SILENCE_TIMEOUT":     settings.faultSilence,
		"DOCTOR_MAX_CONCURRENT":            settings.concurrent,
		"DOCTOR_MAX_CONCURRENT_PER_TARGET": settings.perTarget,
		"DOCTOR_COOKIE_SECURE":             settings.secureCookie,
	}

	checkAgentRanges(t, read)
	var report strings.Builder
	for _, name := range slices.Sorted(maps.Keys(keys)) {
		// 字符串的值从装配结果读取；范围来自同一个变量被错误输入拒绝时的诊断。
		fallback, ok := defaults[name]
		if !ok {
			t.Fatalf("新出现的环境变量 %s 没有默认值断言", name)
		}
		_, problem := loadAgentConfig(read(name, "!"))
		message := "—"
		if problem != nil {
			message = strings.ReplaceAll(problem.Error(), "\n", " | ")
		}
		fmt.Fprintf(&report, "%s\t%v\t%s\n", name, fallback, message)
	}
	want, err := os.ReadFile("testdata/environment.golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	if report.String() != string(want) {
		t.Fatalf("环境配置的名字、默认值或拒绝规则发生变化：\n%s", report.String())
	}
}

// TestDoctorModelEnvironmentContract 两个 API 格式都用虚构凭据做纯配置解析；只有 responses 才需要单列缓存写入价格。
func TestDoctorModelEnvironmentContract(t *testing.T) {
	mockKeys := map[string]bool{}
	_, err := loadAgentConfig(config.NewReader(func(name string) (string, bool) {
		mockKeys[name] = true
		return "", false
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"chat_completions", "responses"} {
		values := map[string]string{"LLM_PROVIDER": "openai", "LLM_BUDGET_ACCOUNT": "test-primary", "LLM_API_FORMAT": format,
			"LLM_BASE_URL": "https://example.invalid", "LLM_API_KEY": "test-only", "LLM_MODEL": "fixture",
			"LLM_INPUT_PRICE_PER_MTOKEN": "1", "LLM_CACHE_HIT_PRICE_PER_MTOKEN": "0.5", "LLM_OUTPUT_PRICE_PER_MTOKEN": "3", "LLM_CACHE_WRITE_PRICE_PER_MTOKEN": "2",
			"EVAL_COMPARISON_ENABLED": "true", "DEEPSEEK_API_KEY": "test-only"}
		// 相邻的允许和拒绝长度都走同一配置解析器。
		for _, tc := range []struct {
			key   string
			size  int
			valid bool
		}{
			{"LLM_BUDGET_ACCOUNT", 64, true}, {"LLM_BUDGET_ACCOUNT", 65, false},
			{"LLM_MODEL", 128, true}, {"LLM_MODEL", 129, false},
		} {
			inputs := maps.Clone(values)
			inputs[tc.key] = strings.Repeat("x", tc.size)
			_, err := loadAgentConfig(config.NewReader(func(name string) (string, bool) {
				v, ok := inputs[name]
				return v, ok
			}))
			if (err == nil) != tc.valid {
				t.Fatalf("%s 长度 %d：%v", tc.key, tc.size, err)
			}
		}
		var specific []string
		reader := config.NewReader(func(name string) (string, bool) {
			// 普通 Mock 路径已有逐项基线。这里只对照启用提供方和比较账户后才出现的键。
			if !mockKeys[name] {
				specific = append(specific, name)
			}
			v, ok := values[name]
			return v, ok
		})
		settings, err := loadAgentConfig(reader)
		if err != nil || settings.comparison == nil {
			t.Fatalf("比较账户配置缺失：%v", err)
		}
		compare := settings.comparison
		slices.Sort(specific)
		// 使用固定日期取得比较账户的价格，避免测试随运行时间改变。
		prices := compare.pricesAt(time.Date(2026, 1, 5, 7, 0, 0, 0, time.UTC))
		result := fmt.Sprintf("%s\n%s %s %s %s %d %d\n%v\n", strings.Join(specific, "\n"),
			compare.model, compare.baseURL, compare.budgetAccount, compare.currency,
			compare.totalMicros, compare.diagnosisMicros, prices)
		want, err := os.ReadFile("testdata/" + format + "_environment.golden.txt")
		if err != nil {
			t.Fatal(err)
		}
		if result != string(want) {
			t.Fatalf("%s 的配置变化：\n%s", format, result)
		}
	}
}

// checkAgentRanges 验证边界值及紧邻的越界值。
func checkAgentRanges(t *testing.T, read func(string, string) config.Reader) {
	t.Helper()
	// 查看该变量自己的范围错误。其他字段之间的预算约束在独立测试中验证。
	checkRange := func(name, raw string, inside bool) {
		t.Helper()
		_, err := loadAgentConfig(read(name, raw))
		rejected := err != nil && strings.Contains(err.Error(), "环境变量 "+name+" 必须是")
		if rejected == inside {
			t.Fatalf("%s=%s，期望在范围内=%t：%v", name, raw, inside, err)
		}
	}
	for _, bound := range []struct {
		name     string
		min, max int
	}{
		{"DOCTOR_MAX_CONCURRENT", 1, 2},
		{"DOCTOR_MAX_CONCURRENT_PER_TARGET", 1, 1},
	} {
		checkRange(bound.name, strconv.Itoa(bound.min-1), false)
		checkRange(bound.name, strconv.Itoa(bound.min), true)
		checkRange(bound.name, strconv.Itoa(bound.max), true)
		checkRange(bound.name, strconv.Itoa(bound.max+1), false)
	}
	for _, bound := range []struct{ name, min, max string }{
		{"DOCTOR_ALERT_PENDING_MAX_AGE", "1s", "1h0m0s"},
		{"DOCTOR_ALERT_SILENCE_TIMEOUT", "3m0s", "24h0m0s"},
		{"DOCTOR_LEASE_TTL", "3s", "1m0s"},
		{"DOCTOR_RESUME_MAX_AGE", "1m0s", "30m0s"},
		{"LLM_TIMEOUT", "1s", "2m0s"},
	} {
		min, err := time.ParseDuration(bound.min)
		if err != nil {
			t.Fatal(err)
		}
		max, err := time.ParseDuration(bound.max)
		if err != nil {
			t.Fatal(err)
		}
		checkRange(bound.name, (min - time.Nanosecond).String(), false)
		checkRange(bound.name, min.String(), true)
		checkRange(bound.name, max.String(), true)
		checkRange(bound.name, (max + time.Nanosecond).String(), false)
	}
}
