package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture 创建独立的假模块，检查器只解析这些文件，不需要下载被禁止的依赖。
func fixture(t *testing.T, path, source string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fullPath := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestForbiddenImports 覆盖领域隔离和分层违规，包括不能靠“包名前缀相似”产生误报的边界。
func TestForbiddenImports(t *testing.T) {
	for _, test := range []struct {
		file      string // file 决定被检查的领域和文件角色。
		imported  string // imported 是要验证的依赖路径。
		forbidden bool   // forbidden 表示本组合应当被拒绝。
	}{
		{"internal/doctor/agent/agent.go", "example.test/project/internal/shop/order", true},
		{"internal/doctor/agent/agent.go", "example.test/project/internal/chaos", true},
		{"internal/doctor/agent/agent.go", "example.test/project/internal/eval/runner", true},
		{"internal/doctor/agent/agent.go", "example.test/project/internal/loadgen", true},
		{"internal/shop/order/repository.go", "example.test/project/internal/doctor/auth", true},
		{"internal/shop/order/repository.go", "example.test/project/internal/eval", true},
		{"internal/shop/order/repository.go", "example.test/project/internal/chaos", true},
		{"internal/platform/config/config.go", "example.test/project/internal/shop/order", true},
		{"internal/platform/config/config.go", "example.test/project/gen/inventory/v1", true},
		{"internal/shop/order/model.go", "net/http", true},
		{"internal/shop/order/service.go", "github.com/gin-gonic/gin", true},
		{"internal/shop/order/service.go", "google.golang.org/grpc/codes", true},
		{"internal/shop/order/service.go", "database/sql", true},
		{"internal/shop/order/service.go", "github.com/redis/go-redis/v9", true},
		{"internal/shop/order/service.go", "github.com/segmentio/kafka-go", true},
		{"internal/shop/order/http_handler.go", "database/sql", true},
		{"internal/shop/order/repository.go", "database/sql", false},
		{"internal/doctor/agent/agent.go", "example.test/project/internal/platform/logging", false},
		{"internal/doctor/agent/agent.go", "example.test/project/internal/shopper", false},
	} {
		t.Run(test.file+test.imported, func(t *testing.T) {
			root := fixture(t, test.file, "package sample\nimport _ \""+test.imported+"\"\n")
			problems, err := check(root)
			if err != nil || (len(problems) != 0) != test.forbidden {
				t.Fatalf("检查结果：%v %v", problems, err)
			}
		})
	}
}

// TestLocations 验证根目录 Go 文件和 cmd 多文件会被拒绝，合法的薄入口不会误报。
func TestLocations(t *testing.T) {
	for _, path := range []string{"main.go", "cmd/demo/extra.go", "cmd/demo/doc.go", "internal/utils/work.go"} {
		if locationProblem(path) == "" {
			t.Errorf("未拒绝 %s", path)
		}
	}
	if locationProblem("cmd/demo/main.go") != "" {
		t.Fatal("合法入口被误报")
	}
}

// TestScenarioReads 验证直接、拼接、Join、常量和 embed 路径都不能从医生包读取标准答案。
func TestScenarioReads(t *testing.T) {
	// 同一个风险使用多种写法；只有直接字面量能被识别的检查很容易被无意绕过。
	for _, source := range []string{
		"package sample\nimport \"os\"\nfunc f(){ os.ReadFile(\"eval/scenarios/S01.yaml\") }",
		"package sample\nimport files \"os\"\nfunc f(){ files.Open(\"eval/\"+\"scenarios/S01.yaml\") }",
		"package sample\nimport (\"os\"; \"path/filepath\")\nfunc f(){ os.ReadFile(filepath.Join(\"eval\",\"scenarios\",\"S01.yaml\")) }",
		"package sample\nimport \"os\"\nconst base = \"eval/scenarios\"\nfunc f(){ os.ReadFile(base+\"/S01.yaml\") }",
		"package sample\nimport \"embed\"\n//go:embed eval/scenarios/*.yaml\nvar files embed.FS",
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
	for _, domain := range []string{"chaos", "eval"} {
		root := fixture(t, "internal/"+domain+"/reader.go", "package sample\nimport \"os\"\nfunc f(){ os.ReadFile(\"eval/scenarios/S01.yaml\") }")
		problems, err := check(root)
		if err != nil || len(problems) != 0 {
			t.Fatalf("授权领域被误报：%v %v", problems, err)
		}
	}
}
