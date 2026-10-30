// 本文件：用真实场景文件和假控制接口验证两种流量检查，所有测试都不花模型费用。
//
// 主要内容：TestScenarioLoadFiles 遍历场景约定；TestLoadExact 拒绝每一种漂移；
// TestPartialLoadActions 验证未填写与显式零不同；TestLoadGuard 验证中途变化会取消。
// TestWorkloadThroughout 验证注入与恢复期间也不能更换工作集或工作者数。
// 阅读建议：测试读取场景答案的能力只属于评测器，不导入医生包。

package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestScenarioLoadFiles 直接读仓库里的全部文件，新增流量动作后也自动进入检查范围。
func TestScenarioLoadFiles(t *testing.T) {
	t.Chdir("../../..")
	values, err := scenario.Load("eval/scenarios")
	if err != nil {
		t.Fatal(err)
	}
	checked := map[string]bool{}
	for _, value := range values {
		t.Run(value.ID, func(t *testing.T) {
			got, err := scenarioLoad(value.ID)
			if err != nil {
				t.Fatal(err)
			}
			// 从后往前查每个字段最后一次赋值，独立核对注入完成时的最终设置。
			want := normalLoad()
			qps, hot := false, false
			for i := len(value.Inject) - 1; i >= 0; i-- {
				a := value.Inject[i]
				if a.Action != "load" {
					continue
				}
				checked[value.ID] = true
				if !qps && a.QPS != nil {
					want.QPS = *a.QPS
					qps = true
				}
				if !hot && a.HotSKUID != nil {
					want.HotSKU = *a.HotSKUID
					hot = true
				}
			}
			if got != want {
				t.Fatalf("场景=%s got=%+v want=%+v", value.ID, got, want)
			}
			reader, closeServer := loadFixture(t, got)
			defer closeServer()
			if err := checkLoad(t.Context(), reader, want); err != nil {
				t.Fatal(err)
			}
		})
	}
	if !checked["S04"] || !checked["S08"] {
		t.Fatal("没有覆盖已知的两种流量场景")
	}
}

// loadFixture 返回完整的运行设置；测试无需启动真正压测器。
func loadFixture(t *testing.T, value loadSettings) (config.Reader, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/control" || r.Method != "GET" || r.Header.Get("Authorization") != "" {
			t.Error("控制请求泄露凭据或路径错误")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"settings": value}})
	}))
	return config.NewReader(func(key string) (string, bool) { return server.URL, key == "LOADGEN_CONTROL_URL" }), server.Close
}

// TestLoadExact 故障期也必须完全相等，而不是允许任意高于正常基线的值。
func TestLoadExact(t *testing.T) {
	want := loadSettings{QPS: 400, HotSKU: 1, Products: 10000, Workers: 64}
	for _, test := range []struct {
		name   string
		actual loadSettings
		valid  bool
	}{
		{"与声明完全一致", want, true},                               // 这才是S08计划的流量。
		{"仍是正常QPS", loadSettings{200, 1, 10000, 64}, false},   // 未真正施加计划中的压力。
		{"更高QPS也不接受", loadSettings{401, 1, 10000, 64}, false}, // 不能把精确匹配改成阈值。
		{"热点不同", loadSettings{400, 2, 10000, 64}, false},      // 错商品可能没有同样的库存与证据。
		{"热点丢失", loadSettings{400, 0, 10000, 64}, false},      // 不能默许回到分散流量。
		{"工作集变化", loadSettings{400, 1, 500000, 64}, false},    // 故障不允许扩大正常工作集。
		{"工作者变化", loadSettings{400, 1, 10000, 63}, false},     // 并发度也是实验条件。
	} {
		t.Run(test.name, func(t *testing.T) {
			reader, closeServer := loadFixture(t, test.actual)
			defer closeServer()
			if err := checkLoad(t.Context(), reader, want); (err == nil) != test.valid {
				t.Fatalf("err=%v", err)
			}
		})
	}
	reader, closeServer := loadFixture(t, want)
	defer closeServer()
	if err := checkBaseline(t.Context(), reader); err == nil {
		t.Fatal("注入前或恢复后误接受故障流量")
	}
}

// TestPartialLoadActions 只改QPS时保留热点，显式零则确实取消热点；动作顺序不能颠倒。
func TestPartialLoadActions(t *testing.T) {
	qps, zero, hot := 400, int64(0), int64(7)
	value := scenario.Scenario{Inject: []scenario.Step{{Action: "load", HotSKUID: &hot}, {Action: "load", QPS: &qps}, {Action: "load", HotSKUID: &zero}}}
	if got := diagnosticLoad(value); got != (loadSettings{400, 0, 10000, 64}) {
		t.Fatal(got)
	}
}

// TestLoadGuard 不对调度耗时作断言，只等待真实的复核周期观察到设置变化。
func TestLoadGuard(t *testing.T) {
	problem := errors.New("场景热点不一致")
	if _, _, err := guardLoad(t.Context(), func(context.Context) error { return problem }); !errors.Is(err, problem) {
		t.Fatal(err)
	}
	var calls atomic.Int32
	// 第一次检查正常，下一次才模拟漂移，证明不是只在入口检查一次。
	ctx, stop, err := guardLoad(t.Context(), func(context.Context) error {
		if calls.Add(1) > 1 {
			return problem
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	<-ctx.Done()
	if !errors.Is(context.Cause(ctx), problem) {
		t.Fatal(fmt.Sprint(context.Cause(ctx)))
	}
}

// TestWorkloadThroughout 过渡中的流量由动作控制，但另两项仍要严格匹配。
func TestWorkloadThroughout(t *testing.T) {
	for _, test := range []struct {
		name   string
		actual loadSettings
		valid  bool
	}{
		{"暂停与热点过渡", loadSettings{0, 1, 10000, 64}, true},  // 注入可能先暂停流量。
		{"工作集漂移", loadSettings{0, 1, 500000, 64}, false},  // 暂停时也不能扩大商品范围。
		{"工作者漂移", loadSettings{400, 1, 10000, 65}, false}, // 场景QPS正确也不能多一个工作者。
	} {
		t.Run(test.name, func(t *testing.T) {
			reader, closeServer := loadFixture(t, test.actual)
			defer closeServer()
			if err := checkWorkload(t.Context(), reader); (err == nil) != test.valid {
				t.Fatal(err)
			}
		})
	}
}
