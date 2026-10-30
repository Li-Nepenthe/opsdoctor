// 本文件：证明流量修订的白名单不能同时更改医生提示、工具、手册或评分。
//
// 主要内容：TestBaselineAmendmentScope 检查源码白名单；TestBaselineBehaviorVersions 保护独立清单字段。
// 阅读建议：只写临时目录中的虚构文件，不动仓库实际输入或Git历史。

package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestBaselineAmendmentScope 把一个允许变化的文件还原后，逐项加入不允许变化的文件。
func TestBaselineAmendmentScope(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, root := range []string{"cmd", "internal", "tools", "gen", "api", "deploy", "eval"} {
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, value string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module test")
	write("go.sum", "")
	write("internal/eval/runner/environment.go", "original")
	old, err := sourceHash()
	if err != nil {
		t.Fatal(err)
	}
	write("internal/eval/runner/environment.go", "fixed")
	// 允许的环境检查恢复旧字节后，完整哈希必须回到同一个值。
	overrides := map[string][]byte{"internal/eval/runner/environment.go": []byte("original")}
	if value, err := sourceHashUsing(overrides); err != nil || value != old {
		t.Fatal(value, err)
	}
	for _, path := range []string{"internal/doctor/agent/system.md", "internal/doctor/tools/registry.go", "deploy/doctor/targets/shop.profile.md", "eval/scenarios/S08.yaml", "internal/eval/judge/rubric.md"} {
		if _, allowed := baselineChanges[path]; allowed {
			t.Fatalf("不该放行 %s", path)
		}
		write(path, "unapproved")
		if value, err := sourceHashUsing(overrides); err != nil || value == old {
			t.Fatalf("未发现 %s: %v", path, err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	// 通用手册单独按内容哈希冻结，不属于sourceHash的目录，白名单也不能接纳它。
	if _, allowed := baselineChanges["docs/runbooks/changes-and-safety.md"]; allowed {
		t.Fatal("允许修改手册")
	}
}

// TestBaselineBehaviorVersions 专门保护源码目录以外的手册哈希，代码版本变动不能顺带换手册。
func TestBaselineBehaviorVersions(t *testing.T) {
	before := json.RawMessage(`{"commit":"old","source_hash":"oldhash","runbooks_hash":"frozen","prompt_version":"v8.0","model":"same"}`)
	for _, test := range []struct {
		name, after string
		valid       bool
	}{
		{"只改执行器版本", `{"commit":"new","source_hash":"newhash","runbooks_hash":"frozen","prompt_version":"v8.0","model":"same"}`, true}, // 仅此项符合授权。
		{"手册改变", `{"commit":"new","source_hash":"newhash","runbooks_hash":"changed","prompt_version":"v8.0","model":"same"}`, false},  // 手册有独立哈希。
		{"提示版本改变", `{"commit":"new","source_hash":"newhash","runbooks_hash":"frozen","prompt_version":"v9","model":"same"}`, false},   // 不能借基础设施修订刷新提示。
		{"模型改变", `{"commit":"new","source_hash":"newhash","runbooks_hash":"frozen","prompt_version":"v8.0","model":"other"}`, false},  // 模型安排同样冻结。
	} {
		t.Run(test.name, func(t *testing.T) {
			if sameBaselineBehavior(before, json.RawMessage(test.after)) != test.valid {
				t.Fatal("版本边界判断错误")
			}
		})
	}
}
