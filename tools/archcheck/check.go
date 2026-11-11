// 本文件：检查源码放置的位置，以及哪些包从场景目录或实验恢复记录读取故障信息。
// 从 check 读起，它遍历仓库并汇总诊断。导入方向和文件角色由 depguard 检查。

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func check(root string) ([]string, error) {
	var problems []string
	// 读取或解析失败会中断检查，避免把漏扫的源码也算作通过。
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
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
		// 仓库规则按正斜杠写，Windows 上也要按同一个相对路径判断。
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			return nil
		}
		if issue := locationProblem(relative); issue != "" {
			problems = append(problems, path+":1: "+issue)
		}
		if filepath.Ext(path) != ".go" {
			if relative == "deploy/docker-compose.yml" {
				issues, err := doctorMountProblems(path)
				if err != nil {
					return err
				}
				problems = append(problems, issues...)
			}
			return nil
		}
		issues, err := inspectSource(path, relative)
		if err != nil {
			return err
		}
		problems = append(problems, issues...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("检查架构：%w", err)
	}
	// CI 和本机按相同的顺序报告问题，方便逐条对照。
	sort.Strings(problems)
	return problems, nil
}

func inspectSource(path, relative string) ([]string, error) {
	// go:embed 出现在注释中，解析时必须保留。
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	return scenarioProblems(relative, file, positions), nil
}

func locationProblem(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 1 && strings.HasSuffix(path, ".go") {
		return "仓库根目录不得放 Go 文件"
	}
	// cmd 根目录的导航文档是唯一例外，程序目录只保留入口。
	if parts[0] == "cmd" && path != "cmd/README.md" && (len(parts) != 3 || parts[2] != "main.go") {
		return "cmd/<程序>/ 只能包含 main.go"
	}
	// 限制的是目录命名，名为 utils.go 的文件不在这条规则里。
	for _, directory := range parts[:len(parts)-1] {
		switch directory {
		case "util", "utils", "common", "helper", "misc", "base":
			return "目录名必须说明职责，不能使用 " + directory
		}
	}
	return ""
}

// 路径前缀必须止于目录边界，避免授权旁边的同名前缀目录。
func under(path, prefix string) bool { return path == prefix || strings.HasPrefix(path, prefix+"/") }

func scenarioProblems(path string, file *ast.File, positions *token.FileSet) []string {
	// 注入器需要场景定义，评测器需要标准答案。
	if under(path, "internal/chaos") || under(path, "internal/eval") {
		return nil
	}
	var problems []string
	imports := importAliases(file)
	values := stringValues(file)
	// 单独出现的路径可能只是说明文字，这里检查传给读取调用的参数。
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !isReadCall(call.Fun, imports) {
			return true
		}
		for _, argument := range call.Args {
			if isScenarioPath(stringExpression(argument, values)) {
				problems = append(problems, positions.Position(call.Pos()).String()+": 只有 internal/chaos 和 internal/eval 可以读取场景或注入状态")
			}
		}
		return true
	})
	for _, group := range file.Comments {
		problems = append(problems, embedProblems(group, positions)...)
	}
	return problems
}

// 编译时嵌入的内容不会经过文件读取调用，因此要单独检查指令里的每个路径。
func embedProblems(group *ast.CommentGroup, positions *token.FileSet) []string {
	var problems []string
	for _, comment := range group.List {
		patterns, found := strings.CutPrefix(comment.Text, "//go:embed ")
		if !found {
			continue
		}
		for _, pattern := range strings.Fields(patterns) {
			if isScenarioPath(strings.Trim(pattern, `"`)) {
				problems = append(problems, positions.Position(comment.Pos()).String()+": 不得嵌入标准答案目录")
			}
		}
	}
	return problems
}

func importAliases(file *ast.File) map[string]string {
	aliases := make(map[string]string)
	for _, item := range file.Imports {
		path, err := strconv.Unquote(item.Path.Value)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		if item.Name != nil {
			name = item.Name.Name
		}
		aliases[name] = path
	}
	return aliases
}

func isReadCall(expression ast.Expr, imports map[string]string) bool {
	// 不管标准库是用原名还是别名导入，都按导入路径辨认读取调用。
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
		// 只有明确来自其他库的同名调用才排除，普通对象也可能持有文件系统。
		return false
	}
	switch name {
	case "ReadFile", "Open", "OpenFile", "ReadDir", "Glob", "Walk", "WalkDir":
		return true
	}
	return false
}

// 只从当前文件推断路径，同名变量共用一项。运行时才确定的部分留为空。
func stringValues(file *ast.File) map[string]string {
	values := make(map[string]string)
	nodes := 0
	ast.Inspect(file, func(ast.Node) bool { nodes++; return true })
	// 后面的声明可能补全前面的引用。每轮只增加已知名字，用节点数作上限。
	for range nodes {
		known := len(values)
		ast.Inspect(file, func(node ast.Node) bool {
			assignStrings(node, values)
			return true
		})
		if len(values) == known {
			break
		}
	}
	return values
}

func assignStrings(node ast.Node, values map[string]string) {
	// 一次调用可能给多个变量赋值，只收集能与右侧表达式按位置对应的名字。
	switch item := node.(type) {
	case *ast.ValueSpec:
		for index := range min(len(item.Names), len(item.Values)) {
			recordString(item.Names[index], item.Values[index], values)
		}
	case *ast.AssignStmt:
		for index := range min(len(item.Lhs), len(item.Rhs)) {
			if name, ok := item.Lhs[index].(*ast.Ident); ok {
				recordString(name, item.Rhs[index], values)
			}
		}
	}
}

func recordString(name *ast.Ident, expression ast.Expr, values map[string]string) {
	// 只认第一个已知值，避免 x = x + suffix 在每次扫描时继续增长。
	value := stringExpression(expression, values)
	if value != "" && values[name.Name] == "" {
		values[name.Name] = value
	}
}

func stringExpression(expression ast.Expr, values map[string]string) string {
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
			// 保留能识别的片段，即使另一端在运行时才能确定。
			return stringExpression(node.X, values) + stringExpression(node.Y, values)
		}
	case *ast.CallExpr:
		if function, ok := node.Fun.(*ast.SelectorExpr); ok && function.Sel.Name == "Join" {
			// 按 Join 的常见写法拼出路径片段。这里没有验证函数是否真的来自 filepath 包。
			parts := make([]string, 0, len(node.Args))
			for _, argument := range node.Args {
				parts = append(parts, stringExpression(argument, values))
			}
			return strings.Join(parts, "/")
		}
	}
	return ""
}

func isScenarioPath(path string) bool {
	// Windows 和 Linux 上的路径写法要指向同一条规则。
	normalized := strings.ReplaceAll(path, "\\", "/")
	normalized = filepath.ToSlash(filepath.Clean(normalized))
	if filepath.Base(normalized) == ".opsdoctor-chaos.json" {
		// 恢复记录包含完整场景，即使换个目录存放，也会泄露故障原因。
		return true
	}
	return normalized == "eval/scenarios" || strings.HasPrefix(normalized, "eval/scenarios/") || strings.Contains(normalized, "/eval/scenarios/") || strings.HasSuffix(normalized, "/eval/scenarios")
}
