// 本文件：按目录、导入和文件访问规则检查架构边界。
//
// check 从 go.mod 识别本模块，再由 inspectSource 分派规则。
// 先读 locationProblem、importProblem 和 layerProblem；最后读 scenarioProblems，了解静态路径分析能发现什么、不能证明什么。

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
// 这里只解析源码，不要求被检查的包先编译成功，便于在编写新包时尽早发现越界依赖。
func check(root string) ([]string, error) {
	// 第 1 步：读取模块路径，才能区分本仓库的业务包和第三方依赖；不能把仓库名字写死。
	module, err := modulePath(root)
	if err != nil {
		return nil, err
	}
	var problems []string
	// 第 2 步：逐个文件检查位置和源码。WalkDir 会把遍历错误交给回调，不能忽略后继续报“通过”。
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "vendor" || entry.Name() == ".obsidian") {
			// 版本数据、第三方源码和个人笔记设置不属于本项目代码；SkipDir 会跳过整个目录。
			return filepath.SkipDir
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		// 规则统一按仓库相对路径和 / 判断，避免 Windows 的反斜杠改变检查结果。
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			// 返回 nil 让 WalkDir 继续走进目录；只有文件才检查“放在哪里”。
			return nil
		}
		if issue := locationProblem(relative); issue != "" {
			// 位置规则也检查非 Go 文件，例如 cmd/demo/README.md 同样不符合“只留 main.go”。
			problems = append(problems, path+":1: "+issue)
		}
		if filepath.Ext(path) != ".go" {
			// 非 Go 文件的位置检查完就结束，不能交给 Go 语法解析器。
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
	// 第 3 步：固定诊断顺序，让本机和 CI 输出便于逐行对照。
	sort.Strings(problems)
	return problems, nil
}

// inspectSource 解析单个 Go 文件并分别检查依赖和答案读取；语法错误不会被当成无违规。
func inspectSource(path, relative, module string) ([]string, error) {
	// AST（抽象语法树）把源码表示成函数、导入、表达式等节点；FileSet 将节点位置换回文件和行号。
	// ParseComments 必须保留注释，因为 go:embed 指令也可能把标准答案读进程序。
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
			// AST 保存的是带引号的源码；解码失败时不能拿错误的路径继续检查依赖。
			return nil, err
		}
		if issue := importProblem(relative, importPath, module); issue != "" {
			// 违规先收集，不中止扫描，用户一次就能看到这个文件里的所有依赖问题。
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
		// Fields 按空白拆分，兼容 module 前后的空格和制表符，也让空行安全地得到空切片。
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "module" {
			// 这里只需要模块名，不解析依赖版本；去掉可选引号后即可和 import 路径比较。
			return strings.Trim(fields[1], `"`), nil
		}
	}
	return "", fmt.Errorf("go.mod 缺少 module 声明")
}

// locationProblem 检查根目录 Go 文件、cmd 入口文件数量和含糊的包名；返回空字符串表示通过。
func locationProblem(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 1 && strings.HasSuffix(path, ".go") {
		// 根目录只放项目配置和文档，程序入口应放到 cmd/<程序>/main.go。
		return "仓库根目录不得放 Go 文件"
	}
	if parts[0] == "cmd" && path != "cmd/README.md" && (len(parts) != 3 || parts[2] != "main.go") {
		// 三段路径必须恰好是 cmd、程序名、main.go；这也拒绝多层子目录和额外文件。
		// cmd 根目录的 README 是分组导航，单独放行；cmd/<程序>/README.md 仍然会被拒绝。
		return "cmd/<程序>/ 只能包含 main.go"
	}
	// 最后一段是文件名，只检查前面的目录，避免把 utils.go 误当成一个含糊的包名。
	for _, directory := range parts[:len(parts)-1] {
		switch directory {
		case "util", "utils", "common", "helper", "misc", "base":
			// 这些名字只表示“杂项”，无法告诉读者该包负责什么，计划书要求用具体职责命名。
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
		// 去掉前缀后发生了变化，才说明这是本模块的依赖；不能把别的仓库同名目录算进来。
		switch {
		case under(file, "internal/doctor"):
			// 医生只能通过约定的外部接口观察靶场，不能直接访问业务实现或故障答案。
			for _, domain := range []string{"shop", "chaos", "eval", "loadgen"} {
				if under(internal, "internal/"+domain) {
					return "doctor 不得依赖 " + domain
				}
			}
		case under(file, "internal/shop"):
			// 靶场要能独立运行，不能依赖诊断、注入或评测程序来完成业务。
			for _, domain := range []string{"doctor", "chaos", "eval"} {
				if under(internal, "internal/"+domain) {
					return "shop 不得依赖 " + domain
				}
			}
		case under(file, "internal/platform"):
			// platform 是通用地基；生成的 inventory 协议也带有业务含义，所以 gen 同样禁止。
			if (under(internal, "internal") && !under(internal, "internal/platform")) || under(internal, "gen") {
				return "platform 不得依赖业务包"
			}
		}
	}
	// 领域方向通过后还要检查文件职责，例如同领域的 service.go 也不能直接导入数据库驱动。
	return layerProblem(file, imported)
}

// layerProblem 按文件角色限制传输和存储依赖；领域流程只应认识接口，不直接认识具体后端。
func layerProblem(file, imported string) string {
	if !under(file, "internal") || under(file, "internal/platform") {
		// 文件分层约束针对业务；工具和 platform 本来就需要直接操作 HTTP、SQL 等技术。
		return ""
	}
	// 用完整包路径判定技术类别，并通过 under 接受它的子包，避免换成 grpc/codes 就漏检。
	name := filepath.Base(file)
	isHTTP := under(imported, "net/http") || under(imported, "github.com/gin-gonic/gin")
	isGRPC := under(imported, "google.golang.org/grpc")
	isSQL := under(imported, "database/sql")
	isRedis := under(imported, "github.com/redis/go-redis") || under(imported, "github.com/go-redis/redis")
	isKafka := under(imported, "github.com/segmentio/kafka-go")
	if (name == "model.go" || name == "service.go") && (isHTTP || isGRPC || isSQL || isRedis || isKafka) {
		// 模型和业务流程只依赖抽象接口，才能在测试里替换数据库或下游服务。
		return name + " 不得依赖传输或存储技术：" + imported
	}
	// 以下文件角色只在 shop CRUD 领域使用；医生不强套相同文件布局。
	if !under(file, "internal/shop") {
		return ""
	}
	if (name == "repository.go" || name == "cache.go" || name == "publisher.go" || name == "consumer.go") && (under(imported, "github.com/gin-gonic/gin") || isGRPC) {
		// 存储和消息文件不应再承担 HTTP/RPC 接口的职责，否则接口细节会混进数据访问代码。
		return name + " 不得导入 Gin 或 gRPC"
	}
	if (name == "http_handler.go" || name == "grpc_server.go") && isSQL {
		// 接口适配层应调用 service；直接写 SQL 会绕过业务规则。
		return name + " 不得直接访问 database/sql"
	}
	if strings.HasSuffix(name, "_client.go") && under(imported, "github.com/gin-gonic/gin") {
		// client 负责向外调用，Gin 负责接收请求；这两个方向的适配不放在同一个文件里。
		return "客户端适配器不得导入 Gin"
	}
	return ""
}

// scenarioProblems 检查文件读取调用和 go:embed；允许 chaos/eval 读取答案，其余包只要可静态识别就报错。
func scenarioProblems(path string, file *ast.File, positions *token.FileSet) []string {
	// 先按调用方所在目录判断权限，再分析读取参数；字符串出现在普通文档里不等于读取。
	if under(path, "internal/chaos") || under(path, "internal/eval") {
		// 注入器需要场景定义，评测器需要标准答案；计划书只授权这两个领域读取该目录。
		return nil
	}
	var problems []string
	imports := importAliases(file)
	values := stringValues(file)
	// Inspect 深度遍历 AST；回调返回 true 表示继续看子节点，false 才表示跳过这一支。
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !isReadCall(call.Fun, imports) {
			// 当前节点不是读取调用，也可能包含嵌套的读取调用，仍要继续向下走。
			return true
		}
		for _, argument := range call.Args {
			// 只根据能从源码推导出的字符串判断，不运行文件里的任何函数。
			if isScenarioPath(stringExpression(argument, values)) {
				problems = append(problems, positions.Position(call.Pos()).String()+": 只有 internal/chaos 和 internal/eval 可以读取 eval/scenarios")
			}
		}
		return true
	})
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "//go:embed ") {
				// embed 在编译时读文件，不会出现在 ReadFile 调用里，必须单独检查注释中的每个模式。
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
			// parser 正常已验证字符串；若仍无法解码，就不登记这个别名，不能猜它属于哪个包。
			continue
		}
		name := filepath.Base(path)
		if item.Name != nil {
			// import files "os" 在源码里叫 files；显式别名要覆盖默认的包名 os。
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
		// os.ReadFile 和 files.Open 都是“对象.方法”形状；先识别左边名字，再查它是否来自 import。
		owner, ok := node.X.(*ast.Ident)
		if !ok {
			// getFS().Open 这类复杂接收者超出当前静态分析范围，不把它猜成标准库调用。
			return false
		}
		name, library = node.Sel.Name, imports[owner.Name]
	case *ast.Ident:
		// 点导入允许直接写 ReadFile；用 imports["."] 找到它可能来自的包。
		name, library = node.Name, imports["."]
	}
	if library != "" && library != "os" && library != "io/fs" && library != "path/filepath" {
		// 已知来自其他包的同名函数不算文件读取；library 为空则仍可能是 fs.FS 对象的方法。
		return false
	}
	switch name {
	case "ReadFile", "Open", "OpenFile", "ReadDir", "Glob", "Walk", "WalkDir":
		// 打开文件、列目录和遍历目录都能接触答案，不能只拦住 ReadFile。
		return true
	}
	return false
}

// stringValues 收集可静态求值的字符串声明与赋值；多轮展开支持文件内的常量引用链。
// 它不执行代码；运行时输入仍是已知边界，应由权限和配置校验兜底。
func stringValues(file *ast.File) map[string]string {
	values := make(map[string]string)
	// 这是“名字 → 已知字符串”的简化表，不区分同名变量的作用域，也不追踪跨文件或运行时赋值。
	// 轮数不超过 AST 节点数，引用循环也会停止；普通常量链通常一两轮就稳定。
	nodes := 0
	ast.Inspect(file, func(ast.Node) bool { nodes++; return true })
	for range nodes {
		changed := false
		assign := func(names []*ast.Ident, expressions []ast.Expr) {
			for index, name := range names {
				if index >= len(expressions) {
					// 如 a, b = f()，名字数多于右侧表达式数；不执行 f()，也不猜第二个返回值。
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
				// const 和 var 声明都可能保存路径，不能只查看函数里的短变量声明。
				assign(item.Names, item.Values)
			case *ast.AssignStmt:
				for index, expression := range item.Lhs {
					if name, ok := expression.(*ast.Ident); ok && index < len(item.Rhs) {
						// 只登记单个变量名，不尝试更新 obj.Path 或 paths[i] 这种对象内部的值。
						assign([]*ast.Ident{name}, []ast.Expr{item.Rhs[index]})
					}
				}
			}
			return true
		})
		if !changed {
			// 这一轮没有发现新字符串，再扫描也不会得到更多信息，提前结束。
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
		// 字面量也可能是数字；只有字符串才按路径处理。
		if node.Kind == token.STRING {
			// parser 已确认字符串语法有效，Unquote 在这里不会失败；去掉引号并还原转义字符。
			value, _ := strconv.Unquote(node.Value)
			return value
		}
	case *ast.Ident:
		// 引用已收集的名字；未识别的名字返回空字符串，表示当前没有可用信息。
		return values[node.Name]
	case *ast.ParenExpr:
		// 括号只改变分组，不改变字符串值，继续查看括号里面的表达式。
		return stringExpression(node.X, values)
	case *ast.BinaryExpr:
		if node.Op == token.ADD {
			// 支持 "eval/" + "scenarios"。未知部分会变成空字符串，因此结果只是已知片段，不保证完整。
			return stringExpression(node.X, values) + stringExpression(node.Y, values)
		}
	case *ast.CallExpr:
		if function, ok := node.Fun.(*ast.SelectorExpr); ok && function.Sel.Name == "Join" {
			// 仅按方法名识别 Join，用 / 合并片段以发现常见路径拼接；不会执行函数或证明它就是 filepath.Join。
			parts := make([]string, 0, len(node.Args))
			for _, argument := range node.Args {
				parts = append(parts, stringExpression(argument, values))
			}
			return strings.Join(parts, "/")
		}
	}
	// 其他表达式可能依赖运行时输入，返回“未知”，不假装这个工具能算出任意程序的结果。
	return ""
}

// isScenarioPath 统一 Windows 分隔符并清理相对路径，识别标准答案目录及其子文件。
func isScenarioPath(path string) bool {
	// 先统一分隔符再消除 ./ 和可折叠的 ../，让两种操作系统上的常见路径写法都可识别。
	normalized := strings.ReplaceAll(path, "\\", "/")
	normalized = filepath.ToSlash(filepath.Clean(normalized))
	// 四种匹配分别覆盖目录本身、相对路径子文件、带前缀的子文件、带前缀的目录；斜杠限定目录边界。
	return normalized == "eval/scenarios" || strings.HasPrefix(normalized, "eval/scenarios/") || strings.Contains(normalized, "/eval/scenarios/") || strings.HasSuffix(normalized, "/eval/scenarios")
}
