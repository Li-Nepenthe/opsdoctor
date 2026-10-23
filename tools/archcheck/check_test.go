// 本文件：用故意违规的假模块检验架构规则。
//
// fixture 只把样例交给 parser，不编译被禁止的导入。
// 先看跨领域依赖，再看 cmd 目录约束和场景答案读取。每个反例都要能给出文件位置，避免检查器只在当前仓库恰巧通过。
//
// 主要内容：
//   - fixture：创建独立的假模块，检查器只解析这些文件，不需要下载被禁止的依赖。
//   - TestForbiddenImports：覆盖领域隔离和分层违规，包括不能靠“包名前缀相似”产生误报的边界。
//   - TestLocations：验证根目录 Go 文件和 cmd 多文件会被拒绝，合法的薄入口不会误报。
//   - TestScenarioReads：验证直接、拼接、Join、常量和 embed 路径都不能从医生包读取标准答案。

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
		// 医生不能导入订单实现，否则可以绕过外部接口直接了解靶场内部。
		{"internal/doctor/agent/agent.go", "example.test/project/internal/shop/order", true},
		// 禁止医生直接调用故障注入器。
		{"internal/doctor/agent/agent.go", "example.test/project/internal/chaos", true},
		// 禁止医生借评测器读取标准答案。
		{"internal/doctor/agent/agent.go", "example.test/project/internal/eval/runner", true},
		// 压测控制也不属于医生可以依赖的实现。
		{"internal/doctor/agent/agent.go", "example.test/project/internal/loadgen", true},
		// 靶场不能反过来依赖医生，即使依赖的是看似通用的认证包。
		{"internal/shop/order/repository.go", "example.test/project/internal/doctor/auth", true},
		// 靶场不能把评测逻辑混入业务。
		{"internal/shop/order/repository.go", "example.test/project/internal/eval", true},
		// 靶场不能依赖注入器才能工作。
		{"internal/shop/order/repository.go", "example.test/project/internal/chaos", true},
		// 通用配置包不能向上依赖订单业务。
		{"internal/platform/config/config.go", "example.test/project/internal/shop/order", true},
		// 生成的库存协议仍含业务概念，platform 也不能导入。
		{"internal/platform/config/config.go", "example.test/project/gen/inventory/v1", true},
		// 模型不能绑定 HTTP 传输细节。
		{"internal/shop/order/model.go", "net/http", true},
		// service 不能依赖 Gin 请求上下文。
		{"internal/shop/order/service.go", "github.com/gin-gonic/gin", true},
		// gRPC 子包同样属于传输依赖，不能靠导入 codes 绕过规则。
		{"internal/shop/order/service.go", "google.golang.org/grpc/codes", true},
		// service 应使用仓储接口，不能直接写 SQL。
		{"internal/shop/order/service.go", "database/sql", true},
		// 带版本后缀的 Redis 客户端也必须被识别。
		{"internal/shop/order/service.go", "github.com/redis/go-redis/v9", true},
		// Kafka 连接细节应留在消息适配层。
		{"internal/shop/order/service.go", "github.com/segmentio/kafka-go", true},
		// HTTP handler 不能绕过 service 直接操作数据库。
		{"internal/shop/order/http_handler.go", "database/sql", true},
		// 对照组：repository 负责 SQL，合法使用不能被拦住。
		{"internal/shop/order/repository.go", "database/sql", false},
		// 对照组：医生可以复用通用日志包。
		{"internal/doctor/agent/agent.go", "example.test/project/internal/platform/logging", false},
		// 只按完整目录边界匹配，shopper 不能因为前缀相同就被当成 shop。
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
