package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// check 从 go.mod 获取真实模块路径后遍历源码；读取或语法错误直接返回，避免把漏检当作通过。
// 只读文件，不加载业务依赖、不访问网络；每次检查使用独立状态，可以并发检查不同目录。
func check(root string) ([]string, error) {
	// 先确定模块路径，再检查每个文件；任何读取失败都中断本次扫描。
	module, err := modulePath(root)
	if err != nil {
		return nil, err
	}
	var problems []string
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "vendor" || entry.Name() == ".obsidian") {
			return filepath.SkipDir
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			return nil
		}
		if issue := locationProblem(relative); issue != "" {
			problems = append(problems, path+":1: "+issue)
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		issues, err := inspectSource(path, relative, module)
		if err != nil {
			return err
		}
		problems = append(problems, issues...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("检查架构：%w", err)
	}
	sort.Strings(problems)
	return problems, nil
}

// inspectSource 解析单个 Go 文件并分别检查依赖和答案读取；语法错误不会被当成无违规。
func inspectSource(path, relative, module string) ([]string, error) {
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var problems []string
	// 先检查导入方向，再分析文件读取；这两类规则互不替代。
	for _, imported := range file.Imports {
		importPath, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			return nil, err
		}
		if issue := importProblem(relative, importPath, module); issue != "" {
			problems = append(problems, positions.Position(imported.Pos()).String()+": "+issue)
		}
	}
	problems = append(problems, scenarioProblems(relative, file, positions)...)
	return problems, nil
}

// modulePath 只读取 go.mod 的 module 行，不要求先下载依赖；缺失或不合法时明确失败。
func modulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("读取 go.mod：%w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "module" {
			return strings.Trim(fields[1], `"`), nil
		}
	}
	return "", fmt.Errorf("go.mod 缺少 module 声明")
}

// locationProblem 检查根目录 Go 文件、cmd 入口文件数量和含糊的包名；返回空字符串表示通过。
func locationProblem(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 1 && strings.HasSuffix(path, ".go") {
		return "仓库根目录不得放 Go 文件"
	}
	if parts[0] == "cmd" && (len(parts) != 3 || parts[2] != "main.go") {
		return "cmd/<程序>/ 只能包含 main.go"
	}
	for _, directory := range parts[:len(parts)-1] {
		switch directory {
		case "util", "utils", "common", "helper", "misc", "base":
			return "目录名必须说明职责，不能使用 " + directory
		}
	}
	return ""
}

// under 按目录边界匹配路径，防止把 internal/doctorish 误当成 internal/doctor。
func under(path, prefix string) bool { return path == prefix || strings.HasPrefix(path, prefix+"/") }

// importProblem 检查领域隔离和包内文件角色；不限制合法的 platform 依赖或消费者声明的接口。
func importProblem(file, imported, module string) string {
	// 先检查本模块内部的领域边界，再把第三方技术依赖交给文件分层规则。
	internal := strings.TrimPrefix(imported, module+"/")
	if internal != imported {
		switch {
		case under(file, "internal/doctor"):
			for _, domain := range []string{"shop", "chaos", "eval", "loadgen"} {
				if under(internal, "internal/"+domain) {
					return "doctor 不得依赖 " + domain
				}
			}
		case under(file, "internal/shop"):
			for _, domain := range []string{"doctor", "chaos", "eval"} {
				if under(internal, "internal/"+domain) {
					return "shop 不得依赖 " + domain
				}
			}
		case under(file, "internal/platform"):
			if (under(internal, "internal") && !under(internal, "internal/platform")) || under(internal, "gen") {
				return "platform 不得依赖业务包"
			}
		}
	}
	return layerProblem(file, imported)
}

// layerProblem 按文件角色限制传输和存储依赖；领域流程只应认识接口，不直接认识具体后端。
func layerProblem(file, imported string) string {
	if !under(file, "internal") || under(file, "internal/platform") {
		return ""
	}
	name := filepath.Base(file)
	isHTTP := under(imported, "net/http") || under(imported, "github.com/gin-gonic/gin")
	isGRPC := under(imported, "google.golang.org/grpc")
	isSQL := under(imported, "database/sql")
	isRedis := under(imported, "github.com/redis/go-redis") || under(imported, "github.com/go-redis/redis")
	isKafka := under(imported, "github.com/segmentio/kafka-go")
	if (name == "model.go" || name == "service.go") && (isHTTP || isGRPC || isSQL || isRedis || isKafka) {
		return name + " 不得依赖传输或存储技术：" + imported
	}
	// 以下文件角色只在 shop CRUD 领域使用；医生不强套相同文件布局。
	if !under(file, "internal/shop") {
		return ""
	}
	if (name == "repository.go" || name == "cache.go" || name == "publisher.go" || name == "consumer.go") && (under(imported, "github.com/gin-gonic/gin") || isGRPC) {
		return name + " 不得导入 Gin 或 gRPC"
	}
	if (name == "http_handler.go" || name == "grpc_server.go") && isSQL {
		return name + " 不得直接访问 database/sql"
	}
	if strings.HasSuffix(name, "_client.go") && under(imported, "github.com/gin-gonic/gin") {
		return "客户端适配器不得导入 Gin"
	}
	return ""
}

// scenarioProblems 检查文件读取调用和 go:embed；允许 chaos/eval 读取答案，其余包只要可静态识别就报错。
func scenarioProblems(path string, file *ast.File, positions *token.FileSet) []string {
	// 先按调用方所在目录判断权限，再分析读取参数；字符串出现在普通文档里不等于读取。
	if under(path, "internal/chaos") || under(path, "internal/eval") {
		return nil
	}
	var problems []string
	imports := importAliases(file)
	values := stringValues(file)
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !isReadCall(call.Fun, imports) {
			return true
		}
		for _, argument := range call.Args {
			if isScenarioPath(stringExpression(argument, values)) {
				problems = append(problems, positions.Position(call.Pos()).String()+": 只有 internal/chaos 和 internal/eval 可以读取 eval/scenarios")
			}
		}
		return true
	})
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "//go:embed ") {
				for _, pattern := range strings.Fields(strings.TrimPrefix(comment.Text, "//go:embed ")) {
					if isScenarioPath(strings.Trim(pattern, `"`)) {
						problems = append(problems, positions.Position(comment.Pos()).String()+": 不得嵌入标准答案目录")
					}
				}
			}
		}
	}
	return problems
}

// importAliases 记录标准库读取包的本地名字；别名导入不能绕过读取检查。
func importAliases(file *ast.File) map[string]string {
	aliases := make(map[string]string)
	for _, item := range file.Imports {
		path, err := strconv.Unquote(item.Path.Value)
		if err != nil {
			continue
		} // parser 已校验字符串，无法解码时该项不会被认作读取包。
		name := filepath.Base(path)
		if item.Name != nil {
			name = item.Name.Name
		}
		aliases[name] = path
	}
	return aliases
}

// isReadCall 识别 os/io/fs/path/filepath 的文件入口；对点导入也检查，避免改导入写法绕过规则。
func isReadCall(expression ast.Expr, imports map[string]string) bool {
	// 既识别标准库入口，也识别 fs.FS 等对象的读取方法；已知标准答案路径应保守拒绝。
	name, library := "", ""
	switch node := expression.(type) {
	case *ast.SelectorExpr:
		owner, ok := node.X.(*ast.Ident)
		if !ok {
			return false
		}
		name, library = node.Sel.Name, imports[owner.Name]
	case *ast.Ident:
		name, library = node.Name, imports["."]
	}
	if library != "" && library != "os" && library != "io/fs" && library != "path/filepath" {
		return false
	}
	switch name {
	case "ReadFile", "Open", "OpenFile", "ReadDir", "Glob", "Walk", "WalkDir":
		return true
	}
	return false
}

// stringValues 收集可静态求值的字符串声明与赋值；多轮展开支持文件内的常量引用链。
// 它不执行代码；运行时输入仍是已知边界，应由权限和配置校验兜底。
func stringValues(file *ast.File) map[string]string {
	values := make(map[string]string)
	// 轮数不超过 AST 节点数，引用循环也会停止；普通常量链通常一两轮就稳定。
	nodes := 0
	ast.Inspect(file, func(ast.Node) bool { nodes++; return true })
	for range nodes {
		changed := false
		assign := func(names []*ast.Ident, expressions []ast.Expr) {
			for index, name := range names {
				if index >= len(expressions) {
					continue
				}
				value := stringExpression(expressions[index], values)
				// 首次解析后不再反复展开，避免 x = x + suffix 这样的运行时循环导致字符串膨胀。
				if value != "" && values[name.Name] == "" {
					values[name.Name] = value
					changed = true
				}
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch item := node.(type) {
			case *ast.ValueSpec:
				assign(item.Names, item.Values)
			case *ast.AssignStmt:
				for index, expression := range item.Lhs {
					if name, ok := expression.(*ast.Ident); ok && index < len(item.Rhs) {
						assign([]*ast.Ident{name}, []ast.Expr{item.Rhs[index]})
					}
				}
			}
			return true
		})
		if !changed {
			break
		}
	}
	return values
}

// stringExpression 展开字符串、标识符、加法和 Join 调用；未知表达式返回空值，不猜测运行时结果。
func stringExpression(expression ast.Expr, values map[string]string) string {
	// 只处理可以由源码直接推导的字符串表达式，不把程序调用当成可执行脚本。
	switch node := expression.(type) {
	case *ast.BasicLit:
		if node.Kind == token.STRING {
			value, _ := strconv.Unquote(node.Value)
			return value
		}
	case *ast.Ident:
		return values[node.Name]
	case *ast.ParenExpr:
		return stringExpression(node.X, values)
	case *ast.BinaryExpr:
		if node.Op == token.ADD {
			return stringExpression(node.X, values) + stringExpression(node.Y, values)
		}
	case *ast.CallExpr:
		if function, ok := node.Fun.(*ast.SelectorExpr); ok && function.Sel.Name == "Join" {
			parts := make([]string, 0, len(node.Args))
			for _, argument := range node.Args {
				parts = append(parts, stringExpression(argument, values))
			}
			return strings.Join(parts, "/")
		}
	}
	return ""
}

// isScenarioPath 统一 Windows 分隔符并清理相对路径，识别标准答案目录及其子文件。
func isScenarioPath(path string) bool {
	normalized := strings.ReplaceAll(path, "\\", "/")
	normalized = filepath.ToSlash(filepath.Clean(normalized))
	return normalized == "eval/scenarios" || strings.HasPrefix(normalized, "eval/scenarios/") || strings.Contains(normalized, "/eval/scenarios/") || strings.HasSuffix(normalized, "/eval/scenarios")
}
