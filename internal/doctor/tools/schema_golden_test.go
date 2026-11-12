// 本文件：固定注册表对外公开的工具定义 JSON，防止换校验器时改变模型输入。
//
// 按原始字节比对，保留模型能看到的字段顺序、零值省略和默认值。
//
// 阅读建议：工具执行函数不会被调用，测试不访问后端或真实排障手册。

package tools

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
)

// schemaGoldenRegistry 包含十六个取证工具和可选的手册检索工具，不连接任何数据源。
func schemaGoldenRegistry() *Registry {
	unused := &unusedSources{}
	registry := New(Dependencies{Metrics: unused, Traces: &struct{ source.TraceSource }{}, Logs: &struct{ source.LogSource }{}, MySQL: unused, Redis: unused, Profiles: unused})
	return registry.WithRunbooks(&Runbooks{})
}

// TestToolDefinitionsGolden 比较完整定义，参数以外的说明文字也属于模型看到的输入。
func TestToolDefinitionsGolden(t *testing.T) {
	got, err := json.Marshal(schemaGoldenRegistry().List())
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/tool_definitions.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		// 只报告首个不同字节，避免长工具菜单淹没真正需要审核的位置。
		first := 0
		for first < min(len(got), len(want)) && got[first] == want[first] {
			first++
		}
		t.Fatalf("工具定义 JSON 在第 %d 字节发生变化：当前 %d 字节，基线 %d 字节；请先审核差异，不要自动更新 golden", first, len(got), len(want))
	}
}
