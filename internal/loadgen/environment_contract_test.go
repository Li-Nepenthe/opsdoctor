// 本文件：用进程自己的配置加载函数固定变量名、默认值和独立校验范围。

package loadgen

import (
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestLoadgenEnvironmentContract 通过压测器配置入口观测默认目标、工作负载和运行时限。
func TestLoadgenEnvironmentContract(t *testing.T) {
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
	settings, err := loadConfig(nil, read("", ""), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defaults := map[string]any{
		"LOADGEN_TARGET":          settings.Target,
		"LOADGEN_ADDR":            settings.ControlAddress,
		"LOADGEN_QPS":             settings.QPS,
		"LOADGEN_PRODUCT_COUNT":   settings.ProductCount,
		"LOADGEN_WORKERS":         settings.Workers,
		"LOADGEN_REQUEST_TIMEOUT": settings.RequestTimeout,
		"LOADGEN_DURATION":        settings.Duration,
	}

	checkLoadgenRanges(t, read)
	var report strings.Builder
	for _, name := range slices.Sorted(maps.Keys(keys)) {
		// 字符串的值从装配结果读取；范围来自同一个变量被错误输入拒绝时的诊断。
		fallback, ok := defaults[name]
		if !ok {
			t.Fatalf("新出现的环境变量 %s 没有默认值断言", name)
		}
		_, problem := loadConfig(nil, read(name, "!"), io.Discard)
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

// checkLoadgenRanges 验证边界值及紧邻的越界值。
func checkLoadgenRanges(t *testing.T, read func(string, string) config.Reader) {
	t.Helper()
	// 查看该变量自己的范围错误。其他字段之间的预算约束在独立测试中验证。
	checkRange := func(name, raw string, inside bool) {
		t.Helper()
		_, err := loadConfig(nil, read(name, raw), io.Discard)
		rejected := err != nil && strings.Contains(err.Error(), "环境变量 "+name+" 必须是")
		if rejected == inside {
			t.Fatalf("%s=%s，期望在范围内=%t：%v", name, raw, inside, err)
		}
	}
	for _, bound := range []struct {
		name     string
		min, max int
	}{
		{"LOADGEN_PRODUCT_COUNT", 1, 500000},
		{"LOADGEN_QPS", 0, 10000},
		{"LOADGEN_WORKERS", 1, 256},
	} {
		checkRange(bound.name, strconv.Itoa(bound.min-1), false)
		checkRange(bound.name, strconv.Itoa(bound.min), true)
		checkRange(bound.name, strconv.Itoa(bound.max), true)
		checkRange(bound.name, strconv.Itoa(bound.max+1), false)
	}
	for _, bound := range []struct{ name, min, max string }{
		{"LOADGEN_DURATION", "0s", "24h0m0s"},
		{"LOADGEN_REQUEST_TIMEOUT", "1ms", "30s"},
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
