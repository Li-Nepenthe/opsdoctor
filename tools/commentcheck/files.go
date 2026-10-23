// 本文件：检查文件说明与包内文件清单，并输出代码地图。
//
// check.go 在解析每个文件后把 AST 交给 describeFile；全部文件读完后再由 inventoryProblems
// 对照 doc.go。把这一层单独放出来，是为了不让函数注释规则和目录导航规则混在一起。
// 先读 describeFile 的三个例外，再读 fileDescription 的格式判断；writeMap 只负责输出已收集的信息。

package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// sourceFile 是地图中的一项，也保留清单校验需要的包说明；这里只保存文本，不执行源码。
type sourceFile struct {
	path        string // path 保留扫描路径，报错时直接定位文件。
	packageKey  string // packageKey 把同目录的内部/外部测试归到被测包。
	summary     string // summary 是文件说明第一句，生成文件使用明确的生成物说明。
	packageText string // packageText 仅 doc.go 使用，供精确匹配文件名。
	generated   bool   // generated 标识自动生成物，不要求手写说明或出现在包清单中。
	command     bool   // command 标识 cmd/<程序>/main.go，那里用包说明代替文件说明。
}

// describeFile 先识别生成物、doc.go 和薄入口，再把普通文件交给严格的文件说明校验。
func describeFile(path string, file *ast.File, positions *token.FileSet) (sourceFile, string) {
	source := sourceFile{path: filepath.ToSlash(path), packageKey: filepath.ToSlash(filepath.Dir(path)) + " (" + strings.TrimSuffix(file.Name.Name, "_test") + ")"}
	source.generated = ast.IsGenerated(file)
	source.command = isCommandMain(path, file)
	if source.generated {
		// 生成代码仍出现在地图中，明确告诉读者回源协议修改，而不是手工补注释。
		source.summary = "工具生成的 Go 代码；阅读对应协议定义，不要手改"
		return source, ""
	}
	if filepath.Base(path) == "doc.go" || source.command {
		// 两个位置已有包说明，不能强行再加一层文件说明；缺包说明由原有规则报错。
		source.summary = firstSentence(file.Doc)
		if file.Doc != nil {
			source.packageText = file.Doc.Text()
		}
		return source, ""
	}
	summary, valid := fileDescription(file, positions)
	source.summary = summary
	if !valid {
		return source, fmt.Sprintf("%s:1:1: 文件说明须以 // 本文件： 开头、写明用途，并与 package 空一行", path)
	}
	return source, ""
}

// fileDescription 要求首行明确说明用途，并与 package 分开；否则 Go 会错误地把它当成包文档。
func fileDescription(file *ast.File, positions *token.FileSet) (string, bool) {
	if len(file.Comments) == 0 {
		return "", false
	}
	description := file.Comments[0]
	first := description.List[0]
	if positions.Position(first.Pos()).Line != 1 || !strings.HasPrefix(first.Text, "// 本文件：") {
		// 只在文件中间提到“本文件”不算导航入口；读者打开文件第一眼就应该知道职责。
		return "", false
	}
	summary := strings.TrimSpace(strings.TrimPrefix(firstSentence(description), "本文件："))
	if summary == "" {
		return "", false
	}
	// 相差至少两行才意味着中间有空行；比较注释组结束位置，支持多行讲解。
	separated := positions.Position(description.End()).Line+1 < positions.Position(file.Package).Line
	return summary, separated
}

// isCommandMain 识别计划书规定的单文件入口；mycmd 或其他目录里的 main.go 不享有例外。
func isCommandMain(path string, file *ast.File) bool {
	return file.Name.Name == "main" && filepath.Base(path) == "main.go" && filepath.Base(filepath.Dir(filepath.Dir(path))) == "cmd"
}

// inventoryProblems 等扫描结束再核对 doc.go，避免因为文件遍历顺序而误报漏项。
func inventoryProblems(sources []sourceFile) []string {
	packageNotes := make(map[string]string)
	for _, source := range sources {
		if filepath.Base(source.path) == "doc.go" {
			packageNotes[source.packageKey] = source.packageText
		}
	}
	var problems []string
	for _, source := range sources {
		if source.generated || source.command || strings.HasSuffix(source.path, "_test.go") {
			continue
		}
		notes := packageNotes[source.packageKey]
		if notes == "" {
			continue
		} // 缺 doc.go 已由包规则定位，不额外重复报一串清单错误。
		name := filepath.Base(source.path)
		if !mentionsFilename(notes, name) {
			problems = append(problems, fmt.Sprintf("%s:1:1: doc.go 的文件清单未提到 %s", source.path, name))
		}
	}
	return problems
}

// mentionsFilename 按完整文件名匹配，避免 old_service.go 或 service.go.bak 冒充 service.go。
func mentionsFilename(notes, name string) bool {
	words := strings.FieldsFunc(notes, func(character rune) bool {
		// 文件名可含字母、数字、下划线、点与短横线；Markdown 标点和中文顿号分隔各项。
		return !unicode.IsLetter(character) && !unicode.IsDigit(character) && !strings.ContainsRune("._-", character)
	})
	for _, word := range words {
		if word == name {
			return true
		}
	}
	return false
}

// writeMap 输出可粘贴到 docs/代码地图.md 的表格；链接以 docs 为起点，统一使用相对路径。
func writeMap(output io.Writer, sources []sourceFile) error {
	sort.Slice(sources, func(left, right int) bool { return sources[left].path < sources[right].path })
	if _, err := fmt.Fprintln(output, "| 目录 | 文件 | 一句话说明 |\n|---|---|---|"); err != nil {
		return err
	}
	for _, source := range sources {
		// 绝对扫描路径也转换成相对路径，避免生成机器专属的 D: 盘链接。
		absolute, err := filepath.Abs(source.path)
		if err != nil {
			return err
		}
		docs, err := filepath.Abs("docs")
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(docs, absolute)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(output, "| %s | [%s](%s) | %s |\n", filepath.ToSlash(filepath.Dir(source.path)), filepath.Base(source.path), filepath.ToSlash(relative), source.summary)
		if err != nil {
			return err
		}
	}
	return nil
}
