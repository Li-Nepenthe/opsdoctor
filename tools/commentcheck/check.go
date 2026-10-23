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
	position    string // position 是文件与一基行号，用户可据此跳到定义。
	summary     string // summary 是文档注释第一句，作为汇报书的清单初稿。
}

// scanResult 累积一次扫描结果；每次调用独占它，不存在跨扫描共享的可变全局数据。
type scanResult struct {
	files    int      // files 是排除生成代码后的 Go 文件数。
	problems []string // problems 是带源码位置的错误列表。
	symbols  []symbol // symbols 是全部类型、函数和方法的清单，包括测试。
}

// scan 遍历 patterns 指定的目录（支持 ./...）；语法错误和读取错误直接返回，不静默漏检。
func scan(patterns []string) (scanResult, error) {
	result := scanResult{}
	seen := make(map[string]bool)
	packages := make(map[string]string)
	documented := make(map[string]bool)
	for _, pattern := range patterns {
		root := strings.TrimSuffix(filepath.ToSlash(pattern), "/...")
		if root == "..." {
			root = "."
		}
		err := filepath.WalkDir(filepath.FromSlash(root), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "vendor" || entry.Name() == ".obsidian") {
				return filepath.SkipDir
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			absolute, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if seen[absolute] {
				return nil
			}
			seen[absolute] = true
			return inspectFile(path, &result, packages, documented)
		})
		if err != nil {
			return result, fmt.Errorf("扫描注释 %s：%w", pattern, err)
		}
	}
	// doc.go 必须含包注释；仅有同名空文件不能冒充包级说明。
	for key, location := range packages {
		if !documented[key] {
			result.problems = append(result.problems, location+": 包缺少带包注释的 doc.go")
		}
	}
	sort.Strings(result.problems)
	return result, nil
}

// inspectFile 解析单个文件并检查声明；标准生成标记由 ast.IsGenerated 识别，生成物跳过。
func inspectFile(path string, result *scanResult, packages map[string]string, documented map[string]bool) error {
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	if ast.IsGenerated(file) {
		return nil
	}
	result.files++
	// 外部测试包 foo_test 使用被测包 foo 的 doc.go，避免要求不可能放在同目录的第二个 doc.go。
	key := filepath.ToSlash(filepath.Dir(path)) + " (" + strings.TrimSuffix(file.Name.Name, "_test") + ")"
	if _, exists := packages[key]; !exists {
		packages[key] = positions.Position(file.Package).String()
	}
	if filepath.Base(path) == "doc.go" && hasDoc(file.Doc) {
		documented[key] = true
	}
	for _, declaration := range file.Decls {
		switch node := declaration.(type) {
		case *ast.FuncDecl:
			inspectFunction(node, file, positions, key, result)
		case *ast.GenDecl:
			inspectDeclaration(node, positions, key, result)
		}
	}
	return nil
}

// inspectFunction 检查文档和长函数内部注释，并记录方法接收者，防止同名方法在清单里混淆。
func inspectFunction(function *ast.FuncDecl, file *ast.File, positions *token.FileSet, key string, result *scanResult) {
	// 方法名附带接收者；随后分别检查声明前的文档和函数体内部的解释。
	name := function.Name.Name
	if function.Recv != nil {
		name = receiverName(function.Recv.List[0].Type) + "." + name
	}
	location := positions.Position(function.Pos()).String()
	if !hasDoc(function.Doc) {
		result.problems = append(result.problems, location+": 函数或方法 "+name+" 缺少文档注释")
	}
	result.symbols = append(result.symbols, symbol{key, name, location, firstSentence(function.Doc)})
	if function.Body == nil {
		return
	}
	lines := positions.Position(function.Body.Rbrace).Line - positions.Position(function.Body.Lbrace).Line - 1
	if lines <= maximumUncommentedLines {
		return
	}
	for _, comment := range file.Comments {
		if comment.Pos() > function.Body.Lbrace && comment.End() < function.Body.Rbrace {
			return
		}
	}
	result.problems = append(result.problems, location+": 函数体超过 15 行但没有内部注释")
}

// receiverName 直接读取 AST 类型表达式；支持指针和泛型接收者，不引入反射或类型加载依赖。
func receiverName(expression ast.Expr) string {
	switch node := expression.(type) {
	case *ast.Ident:
		return node.Name
	case *ast.StarExpr:
		return "*" + receiverName(node.X)
	case *ast.IndexExpr:
		return receiverName(node.X)
	case *ast.IndexListExpr:
		return receiverName(node.X)
	default:
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
				doc = declaration.Doc
			}
			location := positions.Position(node.Pos()).String()
			if !hasDoc(doc) {
				result.problems = append(result.problems, location+": 类型 "+node.Name.Name+" 缺少文档注释")
			}
			result.symbols = append(result.symbols, symbol{key, node.Name.Name, location, firstSentence(doc)})
		case *ast.ValueSpec:
			if hasDoc(declaration.Doc) || hasDoc(node.Doc) || hasDoc(node.Comment) {
				continue
			}
			for _, name := range node.Names {
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
		return ""
	}
	text := strings.TrimSpace(doc.Text())
	if index := strings.IndexAny(text, "\n。"); index >= 0 {
		text = text[:index]
	}
	return strings.ReplaceAll(text, "|", "\\|")
}

// writeList 按包和位置稳定排序，输出 Markdown 表格；写入错误会返回，避免生成不完整的汇报书。
func writeList(output io.Writer, symbols []symbol) error {
	sort.Slice(symbols, func(left, right int) bool {
		if symbols[left].packagePath != symbols[right].packagePath {
			return symbols[left].packagePath < symbols[right].packagePath
		}
		return symbols[left].position < symbols[right].position
	})
	if _, err := fmt.Fprintln(output, "| 包 | 名称 | 位置 | 说明 |\n|---|---|---|---|"); err != nil {
		return err
	}
	for _, item := range symbols {
		if _, err := fmt.Fprintf(output, "| %s | %s | %s | %s |\n", item.packagePath, item.name, filepath.ToSlash(item.position), item.summary); err != nil {
			return err
		}
	}
	return nil
}
