// 本文件：扫描 Go 声明并检查代码层注释。
//
// scan 遍历文件，inspectFile 解析 AST，再分派函数和声明规则；新增文件层规则交给 files.go，避免职责混在一起。
// 先看包说明如何汇总，再读 inspectFunction；-list 的符号也在这次扫描中收集。

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// maximumUncommentedLines 是计划书规定的边界：函数体超过 15 行必须至少有一条内部注释。
const maximumUncommentedLines = 15

// symbol 是 -list 的一行；字段只保存源码信息，不执行或反射被检查的对象。
type symbol struct {
	packagePath string // packagePath 是目录及包名，避免不同目录的同名包混在一起。
	name        string // name 是类型、函数或带接收者的方法名称。
	position    string // position 是文件位置，行号从 1 开始，用户可据此跳到定义。
	summary     string // summary 是文档注释第一句，作为汇报书的清单初稿。
}

// scanResult 把诊断、符号和文件导航放在一次扫描结果中，让三种输出模式使用同一份源码信息。
type scanResult struct {
	files    int          // files 是排除生成代码后的 Go 文件数。
	problems []string     // problems 是带源码位置的错误列表。
	symbols  []symbol     // symbols 是全部类型、函数和方法的清单，包括测试。
	sources  []sourceFile // sources 保留文件说明，用于核对包内清单和生成代码地图。
}

// scan 遍历 patterns 指定的目录（支持 ./...）；语法错误和读取错误直接返回，不静默漏检。
func scan(patterns []string) (scanResult, error) {
	// 第 1 步：本次扫描独占这些表。seen 防止重复检查文件，另外两张表记录包是否有说明。
	result := scanResult{}
	seen := make(map[string]bool)
	packages := make(map[string]string)
	documented := make(map[string]bool)
	for _, pattern := range patterns {
		// ./... 表示从当前目录递归扫描；WalkDir 本身就会递归，所以先去掉这个后缀。
		root := strings.TrimSuffix(filepath.ToSlash(pattern), "/...")
		if root == "..." {
			// 单独输入 ... 也按当前目录处理，避免把它误当成真实目录名。
			root = "."
		}
		err := filepath.WalkDir(filepath.FromSlash(root), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				// 目录读不到就无法确认是否漏了文件，必须报告失败，不能静默跳过。
				return walkErr
			}
			if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "vendor" || entry.Name() == ".obsidian") {
				// 不要求 Git 数据、第三方源码和个人笔记设置遵守本仓库的手写注释规范。
				return filepath.SkipDir
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" {
				// 返回 nil 仍会继续走进目录；只有 Go 文件需要交给语法解析器。
				return nil
			}
			absolute, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if seen[absolute] {
				// 参数可能同时包含 ./... 和某个子目录；用绝对路径避免同一文件重复报错。
				return nil
			}
			seen[absolute] = true
			return inspectFile(path, &result, packages, documented)
		})
		if err != nil {
			return result, fmt.Errorf("扫描注释 %s：%w", pattern, err)
		}
	}
	// 第 2 步：等文件扫描完再核对包说明，否则可能在读到 doc.go 之前就误报缺失。
	// doc.go 必须含包注释；仅有同名空文件不能冒充包级说明。
	for key, location := range packages {
		if !documented[key] {
			result.problems = append(result.problems, location+": 包缺少带包注释的 doc.go（cmd 入口应在 main.go 写包注释）")
		}
	}
	// map 遍历顺序不固定，排序后本机和 CI 才能得到稳定的诊断顺序。
	result.problems = append(result.problems, inventoryProblems(result.sources)...)
	sort.Strings(result.problems)
	return result, nil
}

// inspectFile 解析单个文件并检查声明；标准生成标记由 ast.IsGenerated 识别，生成物跳过。
func inspectFile(path string, result *scanResult, packages map[string]string, documented map[string]bool) error {
	// AST（抽象语法树）把源码分成声明、函数等节点；ParseComments 保留注释，FileSet 负责还原文件和行号。
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	source, problem := describeFile(path, file, positions)
	result.sources = append(result.sources, source)
	if problem != "" {
		result.problems = append(result.problems, problem)
	}
	if ast.IsGenerated(file) {
		// 标准生成标记说明文件由工具维护，手改注释会在下次生成时丢失，因此按计划书豁免。
		return nil
	}
	result.files++
	// 外部测试包 foo_test 使用被测包 foo 的 doc.go，避免要求不可能放在同目录的第二个 doc.go。
	key := filepath.ToSlash(filepath.Dir(path)) + " (" + strings.TrimSuffix(file.Name.Name, "_test") + ")"
	if _, exists := packages[key]; !exists {
		// 每个包保留一个定位点，缺少 doc.go 时只报一次包级错误，避免每个文件都重复报错。
		packages[key] = positions.Position(file.Package).String()
	}
	if packageDocument(path, file) {
		// 普通包要求 doc.go；cmd 按计划书只能有 main.go，所以在入口文件承载同样的包说明。
		documented[key] = true
	}
	for _, declaration := range file.Decls {
		switch node := declaration.(type) {
		case *ast.FuncDecl:
			// 类型分支类似 Java 的 instanceof：取到函数节点后，检查它的文档和函数体。
			inspectFunction(node, file, positions, key, result)
		case *ast.GenDecl:
			// type、const、var、import 共用一类 AST 节点，具体规则交给声明检查函数区分。
			inspectDeclaration(node, positions, key, result)
		}
	}
	return nil
}

// packageDocument 只给 cmd/<程序>/main.go 开入口例外，避免普通包靠随意文件名绕过 doc.go 规则。
func packageDocument(path string, file *ast.File) bool {
	if !hasDoc(file.Doc) {
		return false
	}
	if filepath.Base(path) == "doc.go" {
		return true
	}
	// 查看父目录的父目录而不是包含字符串 cmd，防止 mycmd/foo 或普通 main 包误获豁免。
	return isCommandMain(path, file)
}

// inspectFunction 检查文档和长函数内部注释，并记录方法接收者，防止同名方法在清单里混淆。
func inspectFunction(function *ast.FuncDecl, file *ast.File, positions *token.FileSet, key string, result *scanResult) {
	// 方法名附带接收者；随后分别检查声明前的文档和函数体内部的解释。
	name := function.Name.Name
	if function.Recv != nil {
		// 有接收者的是方法，例如 (*Reader).String；附上类型名才能与其他类型的同名方法区分。
		name = receiverName(function.Recv.List[0].Type) + "." + name
	}
	location := positions.Position(function.Pos()).String()
	if !hasDoc(function.Doc) {
		// 文档必须紧邻函数声明；函数体里的说明不能替代它对调用方的使用说明。
		result.problems = append(result.problems, location+": 函数或方法 "+name+" 缺少文档注释")
	}
	result.symbols = append(result.symbols, symbol{key, name, location, firstSentence(function.Doc)})
	if function.Body == nil {
		// 有些函数由汇编等外部代码实现，Go 文件里只有声明；仍要求文档，但无函数体可检查。
		return
	}
	// 去掉左右花括号所在行，按物理行数计算；这是计划书的简单门槛，不是代码复杂度分析。
	lines := positions.Position(function.Body.Rbrace).Line - positions.Position(function.Body.Lbrace).Line - 1
	if lines <= maximumUncommentedLines {
		// 恰好 15 行还不触发长函数规则；“超过 15 行”从第 16 行开始。
		return
	}
	for _, comment := range file.Comments {
		if comment.Pos() > function.Body.Lbrace && comment.End() < function.Body.Rbrace {
			// 注释必须完整落在函数体内；找到一条就满足机器规则，解释质量仍要人工读。
			return
		}
	}
	result.problems = append(result.problems, location+": 函数体超过 15 行但没有内部注释")
}

// receiverName 直接读取 AST 类型表达式；支持指针和泛型接收者，不引入反射或类型加载依赖。
func receiverName(expression ast.Expr) string {
	switch node := expression.(type) {
	case *ast.Ident:
		// T 这样的普通接收者直接使用类型名。
		return node.Name
	case *ast.StarExpr:
		// *T 是指针接收者，保留 * 提醒读者方法接收的是指针，再递归取出 T。
		return "*" + receiverName(node.X)
	case *ast.IndexExpr:
		// 泛型接收者 T[A] 只展示基础类型 T，参数 A 不影响这里按类型区分方法。
		return receiverName(node.X)
	case *ast.IndexListExpr:
		// T[A, B] 对应另一个节点类型，和单参数情况一样取 T。
		return receiverName(node.X)
	default:
		// 遇到未支持的类型形状仍保留方法条目，用占位名避免清单生成过程崩溃。
		return "receiver"
	}
}

// inspectDeclaration 检查包级类型、常量、变量；常量和变量允许组注释，局部变量不在机器检查范围。
func inspectDeclaration(declaration *ast.GenDecl, positions *token.FileSet, key string, result *scanResult) {
	// import 是依赖声明，不是需要逐项写文档的类型或包级状态。
	if declaration.Tok == token.IMPORT {
		return
	}
	for _, specification := range declaration.Specs {
		switch node := specification.(type) {
		case *ast.TypeSpec:
			doc := node.Doc
			if doc == nil && len(declaration.Specs) == 1 {
				// 单独的 type T 声明，其注释可能挂在外层 GenDecl；一组多个类型则必须分别说明。
				doc = declaration.Doc
			}
			location := positions.Position(node.Pos()).String()
			if !hasDoc(doc) {
				// 类型表达不同概念，不能拿一段宽泛的组注释替所有类型交差。
				result.problems = append(result.problems, location+": 类型 "+node.Name.Name+" 缺少文档注释")
			}
			result.symbols = append(result.symbols, symbol{key, node.Name.Name, location, firstSentence(doc)})
		case *ast.ValueSpec:
			if hasDoc(declaration.Doc) || hasDoc(node.Doc) || hasDoc(node.Comment) {
				// 常量和变量允许组说明、项目前说明或行尾说明；任一种存在就符合这一条规则。
				continue
			}
			for _, name := range node.Names {
				// var a, b int 会声明多个名字，分别报告，读者才知道哪些包级状态缺少解释。
				result.problems = append(result.problems, positions.Position(name.Pos()).String()+": 包级常量或变量 "+name.Name+" 缺少注释")
			}
		}
	}
}

// hasDoc 不把空白注释或单独的 go: 指令当成说明，防止只有形式却没有可读内容。
func hasDoc(doc *ast.CommentGroup) bool { return doc != nil && strings.TrimSpace(doc.Text()) != "" }

// firstSentence 以换行或中文句号结束第一句；清单是初稿，不替代完整文档注释。
func firstSentence(doc *ast.CommentGroup) string {
	if !hasDoc(doc) {
		// 缺少注释的声明仍会列入阅读清单，只把摘要留空；报错由前面的检查负责。
		return ""
	}
	text := strings.TrimSpace(doc.Text())
	if index := strings.IndexAny(text, "\n。"); index >= 0 {
		// 清单只放第一句，完整解释留在源码里，避免一个表格单元格塞进整段文档。
		text = text[:index]
	}
	// Markdown 用 | 分列，正文里的竖线必须转义，否则会破坏汇报书表格。
	return strings.ReplaceAll(text, "|", "\\|")
}

// writeList 按包和位置稳定排序，输出 Markdown 表格；写入错误会返回，避免生成不完整的汇报书。
func writeList(output io.Writer, symbols []symbol) error {
	// 第 1 步：先按包、再按位置文本排序；顺序稳定即可，这里没有把行号单独按整数排序。
	sort.Slice(symbols, func(left, right int) bool {
		if symbols[left].packagePath != symbols[right].packagePath {
			return symbols[left].packagePath < symbols[right].packagePath
		}
		return symbols[left].position < symbols[right].position
	})
	if _, err := fmt.Fprintln(output, "| 包 | 名称 | 位置 | 说明 |\n|---|---|---|---|"); err != nil {
		// 输出可能是文件或管道，写入失败就返回，避免把不完整表格当成成功结果。
		return err
	}
	// 第 2 步：每个符号写一行，位置中的斜杠统一成 GitHub 和 Windows 都容易阅读的形式。
	for _, item := range symbols {
		if _, err := fmt.Fprintf(output, "| %s | %s | %s | %s |\n", item.packagePath, item.name, filepath.ToSlash(item.position), item.summary); err != nil {
			return err
		}
	}
	return nil
}
