// 本文件：验证工具统一入口的参数、超时、截断和系统绑定。
//
// 假数据源只记录自己有没有被调用，不连接任何服务。超时用标准库虚拟时钟，
// 所以检查完整十秒预算也不用让测试真的等十秒。
//
// 主要内容：
//   - TestArguments：未知字段、错误类型、多份 JSON 和危险 SQL 都在数据源前被挡住。
//   - TestLimits：十秒预算、父级取消、中文截断和错误恢复。
//   - TestBoundSources：两份注册表互不借用数据源，公开说明不能修改执行规则。
//
// 阅读建议：先看调用计数断言，再看结果的实际 JSON 大小断言。

package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/synctest"
	"time"
	"unicode/utf8"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
)

// fakeMetrics 为每个实例保留独立计数，测试才能发现工具误用了另一个系统的数据源。
type fakeMetrics struct {
	calls int    // 实际查询次数。
	value string // 每个系统各自的假指标值。
	wait  bool   // 为 true 时一直等调用方取消。
}

// Query 不读取表达式中的任何地址，wait 用来检查截止时间是否真的传到了数据源。
func (fake *fakeMetrics) Query(ctx context.Context, _ source.MetricQuery) (source.MetricResult, error) {
	fake.calls++
	if fake.wait {
		<-ctx.Done()
		return source.MetricResult{}, ctx.Err()
	}
	return source.MetricResult{Series: []source.Series{{Labels: map[string]string{"service": "orders"}, Points: []source.Point{{Time: 1, Value: fake.value}}}}}, nil
}

// Alerts 返回正常空结果，避免把没有告警当成数据源故障。
func (fake *fakeMetrics) Alerts(context.Context) ([]source.Alert, error) {
	return []source.Alert{}, nil
}

// TestArguments 检查错误参数不会触碰后端，尤其不能多塞一个 URL 或系统名。
func TestArguments(t *testing.T) {
	for _, testcase := range []struct {
		name string // 本条用例要守住的边界。
		args string // 传给统一入口的原始 JSON。
	}{
		{"URL字段", `{"query":"up","url":"http://elsewhere.test"}`},
		{"主机字段", `{"query":"up","host":"elsewhere.test"}`},
		{"系统字段", `{"query":"up","target":"another"}`},
		{"系统别名字段", `{"query":"up","system":"another"}`},
		{"嵌套地址", `{"query":{"url":"http://elsewhere.test"}}`},
		{"重复字段", `{"query":"up","query":"other"}`},
		{"多份JSON", `{"query":"up"} {}`},
		{"数组", `[]`},
		{"null", `null`},
		{"缺必填", `{}`},
		{"数字字符串", `{"query":"up","step_seconds":"10"}`},
		{"小数", `{"query":"up","step_seconds":1.5}`},
		{"范围越界", `{"query":"up","lookback_seconds":3601}`},
		{"点数越界", `{"query":"up","lookback_seconds":3600,"step_seconds":1}`},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			backend := &fakeMetrics{value: "1"}
			registry := New(Dependencies{Metrics: backend})
			result := registry.Run(t.Context(), "query_metrics", json.RawMessage(testcase.args))
			if result.Status != "invalid_argument" || backend.calls != 0 {
				t.Fatalf("参数绕过：%+v calls=%d", result, backend.calls)
			}
		})
	}
}

// TestLimits 的超时断言使用精确虚拟时间，不把调度快慢混进预算规则。
func TestLimits(t *testing.T) {
	// 虚拟时钟让等待只推进测试时间，不依赖电脑是否恰好在一毫秒内调度到这个 goroutine。
	t.Run("完整十秒", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			registry := New(Dependencies{Metrics: &fakeMetrics{wait: true}})
			started := time.Now()
			result := registry.Run(t.Context(), "query_metrics", json.RawMessage(`{"query":"up"}`))
			if result.Status != "timeout" || time.Since(started) != 10*time.Second {
				t.Fatalf("预算：%+v %s", result, time.Since(started))
			}
		})
	})
	t.Run("更短的调用方预算", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			registry := New(Dependencies{Metrics: &fakeMetrics{wait: true}})
			started := time.Now()
			result := registry.Run(ctx, "query_metrics", json.RawMessage(`{"query":"up"}`))
			if result.Status != "timeout" || time.Since(started) != time.Second {
				t.Fatalf("短预算被延长：%+v", result)
			}
		})
	})
	t.Run("已取消", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		result := New(Dependencies{Metrics: &fakeMetrics{wait: true}}).Run(ctx, "query_metrics", json.RawMessage(`{"query":"up"}`))
		if result.Status != "cancelled" {
			t.Fatalf("取消：%+v", result)
		}
	})
	t.Run("中文与转义截断", func(t *testing.T) {
		registry := New(Dependencies{Profile: strings.Repeat("库存\n\"说明", 3000)})
		result := registry.Run(t.Context(), "service_topology", json.RawMessage(`{}`))
		data, err := json.Marshal(result)
		if err != nil || !json.Valid(data) || !utf8.Valid(data) || !result.Truncated || len(data) > 4096 || result.Bytes != len(data) || result.OriginalBytes <= 4096 {
			t.Fatalf("结果限制失败：bytes=%d result=%+v err=%v", len(data), result, err)
		}
		if !strings.Contains(result.Summary, "不能视为全量") {
			t.Fatal("未说明截断限制")
		}
	})
	t.Run("内部panic", func(t *testing.T) {
		registry := New(Dependencies{})
		item := registry.tools["service_topology"]
		item.run = func(context.Context, arguments) (output, error) { panic("secret-must-not-escape") }
		registry.tools["service_topology"] = item
		result := registry.Run(t.Context(), "service_topology", json.RawMessage(`{}`))
		data, _ := json.Marshal(result)
		if result.Status != "internal" || strings.Contains(string(data), "secret-must-not-escape") {
			t.Fatalf("panic 泄露：%s", data)
		}
	})
}

// TestBoundSources 同样的工具名在两个系统里仍使用各自构造时绑定的对象。
func TestBoundSources(t *testing.T) {
	a, b := &fakeMetrics{value: "11"}, &fakeMetrics{value: "22"}
	first, second := New(Dependencies{Metrics: a}), New(Dependencies{Metrics: b})
	for _, item := range []struct {
		registry *Registry
		expected string
	}{{first, "11"}, {second, "22"}} {
		result := item.registry.Run(t.Context(), "query_metrics", json.RawMessage(`{"query":"up"}`))
		if result.Status != "ok" || !strings.Contains(string(result.Data), `"last":"`+item.expected+`"`) {
			t.Fatalf("系统结果混用：%+v", result)
		}
	}
	if a.calls != 1 || b.calls != 1 {
		t.Fatalf("系统调用次数串了：%d %d", a.calls, b.calls)
	}
	// List 返回副本。改公开说明不能把 URL 参数加到运行时规则里。
	for _, definition := range first.List() {
		definition.Parameters.Properties["url"] = textProperty("bad", 0, 100)
	}
	result := first.Run(t.Context(), "query_metrics", json.RawMessage(`{"query":"up","url":"http://example.test"}`))
	if result.Status != "invalid_argument" || a.calls != 1 {
		t.Fatal("公开说明修改了真实校验")
	}
}
