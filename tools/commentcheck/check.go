// 本文件：扫描源码，检查包说明和导出声明的注释，同时收集阅读清单。
// 从 scan 进入。多个目录可能指向同一个文件，检查和清单都只收它一次。

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

// symbol 记录阅读清单中的一个类型、函数或方法，包括测试和未导出的声明。
type symbol struct {
	packagePath string
	name        string
	position    string
	summary     string // 没写注释的声明也列出，摘要留空。
}

type scanResult struct {
	files    int // 不含生成文件；代码地图仍保留它们。
	problems []string
	symbols  []symbol
	sources  []sourceFile
}

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
				// 第三方依赖和本地编辑器配置不属于项目源码。
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
	// 扫完整个包后再报缺少包说明，避免把后面才会读到的 doc.go 算作缺失。
	for key, location := range packages {
		if !documented[key] {
			result.problems = append(result.problems, location+": 包缺少带包注释的 doc.go（cmd 入口应在 main.go 写包注释）")
		}
	}
	// 输出顺序固定，同一组问题在本机和 CI 中可以逐行对照。
	result.problems = append(result.problems, inventoryProblems(result.sources)...)
	sort.Strings(result.problems)
	return result, nil
}

func inspectFile(path string, result *scanResult, packages map[string]string, documented map[string]bool) error {
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
		// 生成文件保留在地图里，声明由生成器维护。
		return nil
	}
	result.files++
	// 同一目录的外部测试包和被测包共用一份包说明。
	key := filepath.ToSlash(filepath.Dir(path)) + " (" + strings.TrimSuffix(file.Name.Name, "_test") + ")"
	if _, exists := packages[key]; !exists {
		packages[key] = positions.Position(file.Package).String()
	}
	if packageDocument(path, file) {
		documented[key] = true
	}
	// 测试文件同样进入阅读清单，仅跳过声明的文档注释检查。
	checkDocs := !strings.HasSuffix(path, "_test.go")
	for _, declaration := range file.Decls {
		switch node := declaration.(type) {
		case *ast.FuncDecl:
			inspectFunction(node, positions, key, result, checkDocs)
		case *ast.GenDecl:
			inspectDeclaration(node, positions, key, result, checkDocs)
		}
	}
	return nil
}

func packageDocument(path string, file *ast.File) bool {
	if !hasDoc(file.Doc) {
		return false
	}
	if filepath.Base(path) == "doc.go" {
		return true
	}
	// 只有 cmd/<程序>/main.go 可以用入口文件的说明代替 doc.go。
	return isCommandMain(path, file)
}

func inspectFunction(function *ast.FuncDecl, positions *token.FileSet, key string, result *scanResult, checkDocs bool) {
	name := function.Name.Name
	if function.Recv != nil {
		// 在清单里区分不同类型的同名方法。
		name = receiverName(function.Recv.List[0].Type) + "." + name
	}
	location := positions.Position(function.Pos()).String()
	if checkDocs && ast.IsExported(function.Name.Name) && !hasDoc(function.Doc) {
		result.problems = append(result.problems, location+": 函数或方法 "+name+" 缺少文档注释")
	}
	result.symbols = append(result.symbols, symbol{key, name, location, firstSentence(function.Doc)})
}

// 清单保留指针接收者；泛型方法按接收者的类型名归在一起。
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

func inspectDeclaration(declaration *ast.GenDecl, positions *token.FileSet, key string, result *scanResult, checkDocs bool) {
	if declaration.Tok == token.IMPORT {
		return
	}
	for _, specification := range declaration.Specs {
		switch node := specification.(type) {
		case *ast.TypeSpec:
			doc := node.Doc
			if doc == nil && len(declaration.Specs) == 1 {
				// 单独声明的类型可用声明前的说明；一个组里有多个类型时，各自需要说明。
				doc = declaration.Doc
			}
			location := positions.Position(node.Pos()).String()
			if checkDocs && ast.IsExported(node.Name.Name) && !hasDoc(doc) {
				result.problems = append(result.problems, location+": 类型 "+node.Name.Name+" 缺少文档注释")
			}
			result.symbols = append(result.symbols, symbol{key, node.Name.Name, location, firstSentence(doc)})
		case *ast.ValueSpec:
			if !checkDocs || hasDoc(declaration.Doc) || hasDoc(node.Doc) || hasDoc(node.Comment) {
				// 包级常量和变量可以共用组说明，也可以各自写在声明前或行末。
				continue
			}
			for _, name := range node.Names {
				if ast.IsExported(name.Name) {
					result.problems = append(result.problems, positions.Position(name.Pos()).String()+": 包级常量或变量 "+name.Name+" 缺少注释")
				}
			}
		}
	}
}

// 空行或单独的编译指令不能充当声明的说明。
func hasDoc(doc *ast.CommentGroup) bool { return doc != nil && strings.TrimSpace(doc.Text()) != "" }

func firstSentence(doc *ast.CommentGroup) string {
	if !hasDoc(doc) {
		return ""
	}
	text := strings.TrimSpace(doc.Text())
	if index := strings.IndexAny(text, "\n。"); index >= 0 {
		text = text[:index]
	}
	// 清单是 Markdown 表格，注释里的竖线应留在同一格。
	return strings.ReplaceAll(text, "|", "\\|")
}

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
