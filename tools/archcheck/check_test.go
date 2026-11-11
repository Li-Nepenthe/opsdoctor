// 本文件：用临时源码检验文件位置和标准答案读取规则。
//
// 导入规则的验证由 depguard 负责；这里保留 cmd 目录约束和场景答案读取的反例。
// 每个读取反例都要能给出文件位置。

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture 创建独立的源码目录，检查器只解析文件，不编译或运行样例。
func fixture(t *testing.T, path, source string) string {
	t.Helper()
	root := t.TempDir()
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestLocations 验证根目录 Go 文件和 cmd 多文件会被拒绝，合法的薄入口不会误报。
func TestLocations(t *testing.T) {
	for _, path := range []string{
		"main.go",                // 根目录不允许放 Go 入口。
		"cmd/demo/extra.go",      // 一个 cmd 程序目录不能再塞第二份实现。
		"cmd/demo/doc.go",        // doc.go 也不能例外，入口说明应写在 main.go。
		"cmd/demo/README.md",     // 例外只给 cmd 根目录，不能扩大到程序目录。
		"internal/utils/work.go", // 包名必须说明职责，不能叫 utils。
	} {
		if locationProblem(path) == "" {
			t.Errorf("未拒绝 %s", path)
		}
	}
	if locationProblem("cmd/demo/main.go") != "" {
		t.Fatal("合法入口被误报")
	}
	if locationProblem("cmd/README.md") != "" {
		t.Fatal("cmd 分组目录的 README 被误报")
	}
}

// TestScenarioReads 验证直接、拼接、Join、常量和 embed 路径都不能从医生包读取标准答案。
func TestScenarioReads(t *testing.T) {
	// 同一个风险使用多种写法。只有直接字面量能被识别的检查很容易被无意绕过。
	for _, source := range []string{
		// 直接传入答案路径，是最基本的读取违规。
		"package sample\nimport \"os\"\nfunc f(){ os.ReadFile(\"eval/scenarios/S01.yaml\") }",
		// 同时改包别名并用 + 拼接，仍然属于同一种读取。
		"package sample\nimport files \"os\"\nfunc f(){ files.Open(\"eval/\"+\"scenarios/S01.yaml\") }",
		// 常见的 filepath.Join 拼接也要能识别。
		"package sample\nimport (\"os\"; \"path/filepath\")\nfunc f(){ os.ReadFile(filepath.Join(\"eval\",\"scenarios\",\"S01.yaml\")) }",
		// 路径前缀放在常量里，读取调用处只引用名字，不能因此漏掉。
		"package sample\nimport \"os\"\nconst base = \"eval/scenarios\"\nfunc f(){ os.ReadFile(base+\"/S01.yaml\") }",
		// embed 在编译时读文件。没有运行时的读取函数也必须拒绝。
		"package sample\nimport \"embed\"\n//go:embed eval/scenarios/*.yaml\nvar files embed.FS",
		// 注入器的恢复记录里也包含场景，不能只防标准答案目录。
		"package sample\nimport \"os\"\nfunc f(){ os.ReadFile(\".opsdoctor-chaos.json\") }",
		// 上级目录和 Join 都不能让状态文件变成医生可读的普通配置。
		"package sample\nimport (\"os\"; \"path/filepath\")\nfunc f(){ os.ReadFile(filepath.Join(\"..\",\".opsdoctor-chaos.json\")) }",
	} {
		root := fixture(t, "internal/doctor/agent/agent.go", source)
		problems, err := check(root)
		if err != nil || len(problems) == 0 {
			t.Fatalf("未发现标准答案读取：%s / %v / %v", source, problems, err)
		}
		if !strings.Contains(problems[0], ":") {
			t.Fatal("诊断没有位置")
		}
	}
	const readProblem = ": 只有 internal/chaos 和 internal/eval 可以读取场景或注入状态"
	for _, tc := range []struct {
		name, source, want string
	}{
		{"quoted_embed", `package sample
import _ "embed"
//go:embed "eval/scenarios/a.yaml"
var scenario string
`, ": 不得嵌入标准答案目录"},
		{"local_assignment", `package sample
import "os"
func f() {
	p := "eval/scenarios/a.yaml"
	os.ReadFile(p)
}
`, readProblem},
		{"forward_reference", `package sample
import "os"
var p = q
var q = "eval/scenarios"
func f() { os.ReadFile(p) }
`, readProblem},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t, "internal/doctor/agent/agent.go", tc.source)
			problems, err := check(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(problems) != 1 || !strings.HasSuffix(problems[0], tc.want) {
				t.Fatalf("期望一条以 %q 结尾的诊断，实际得到 %v", tc.want, problems)
			}
		})
	}
	for _, domain := range []string{
		"chaos", // 注入器需要按场景定义制造和恢复故障，允许读取。
		"eval",  // 评测器需要标准答案来评分，允许读取。
	} {
		root := fixture(t, "internal/"+domain+"/reader.go", "package sample\nimport \"os\"\nfunc f(){ os.ReadFile(\"eval/scenarios/S01.yaml\") }")
		problems, err := check(root)
		if err != nil || len(problems) != 0 {
			t.Fatalf("授权领域被误报：%v %v", problems, err)
		}
	}
}
