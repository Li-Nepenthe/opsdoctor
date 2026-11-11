// 本文件：核对文件说明与包内文件清单，并生成代码地图。
// 从 describeFile 看每项说明的出处。等同一个包的文件全部收齐，再与 doc.go 对照。

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

type sourceFile struct {
	path        string
	packageKey  string // 外部测试和被测包归在同一个目录。
	summary     string
	packageText string // doc.go 的全文，核对文件名时第一句不够。
	generated   bool
	command     bool
}

func describeFile(path string, file *ast.File, positions *token.FileSet) (sourceFile, string) {
	source := sourceFile{path: filepath.ToSlash(path), packageKey: filepath.ToSlash(filepath.Dir(path)) + " (" + strings.TrimSuffix(file.Name.Name, "_test") + ")"}
	source.generated = ast.IsGenerated(file)
	source.command = isCommandMain(path, file)
	if source.generated {
		source.summary = "工具生成的 Go 代码；阅读对应协议定义，不要手改"
		return source, ""
	}
	if filepath.Base(path) == "doc.go" || source.command {
		// 地图沿用包说明，缺少包说明由包级检查统一报告。
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

func fileDescription(file *ast.File, positions *token.FileSet) (string, bool) {
	if len(file.Comments) == 0 {
		return "", false
	}
	description := file.Comments[0]
	first := description.List[0]
	if positions.Position(first.Pos()).Line != 1 || !strings.HasPrefix(first.Text, "// 本文件：") {
		return "", false
	}
	summary := strings.TrimSpace(strings.TrimPrefix(firstSentence(description), "本文件："))
	if summary == "" {
		return "", false
	}
	// 多行说明以最后一行计算，避免紧贴 package 的包注释被当作文件说明。
	separated := positions.Position(description.End()).Line+1 < positions.Position(file.Package).Line
	return summary, separated
}

func isCommandMain(path string, file *ast.File) bool {
	return file.Name.Name == "main" && filepath.Base(path) == "main.go" && filepath.Base(filepath.Dir(filepath.Dir(path))) == "cmd"
}

func inventoryProblems(sources []sourceFile) []string {
	packageNotes := make(map[string]string)
	for _, source := range sources {
		if filepath.Base(source.path) == "doc.go" {
			packageNotes[source.packageKey] = source.packageText
		}
	}
	var problems []string
	for _, source := range sources {
		name := filepath.Base(source.path)
		if name == "doc.go" || source.generated || source.command || strings.HasSuffix(name, "_test.go") {
			continue
		}
		// 如果整个包都没有说明，由包级检查报一次，避免每个文件再报一次。
		notes := packageNotes[source.packageKey]
		if notes == "" {
			continue
		}
		if !mentionsFilename(notes, name) {
			problems = append(problems, fmt.Sprintf("%s:1:1: doc.go 的文件清单未提到 %s", source.path, name))
		}
	}
	return problems
}

// 用完整文件名核对，防止 old_service.go 或 service.go.bak 被算成 service.go。
func mentionsFilename(notes, name string) bool {
	words := strings.FieldsFunc(notes, func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character) && !strings.ContainsRune("._-", character)
	})
	for _, word := range words {
		if word == name {
			return true
		}
	}
	return false
}

func writeMap(output io.Writer, sources []sourceFile) error {
	sort.Slice(sources, func(left, right int) bool { return sources[left].path < sources[right].path })
	if _, err := fmt.Fprintln(output, "| 目录 | 文件 | 一句话说明 |\n|---|---|---|"); err != nil {
		return err
	}
	for _, source := range sources {
		// 地图保存在 docs，所有链接都从那里出发，换台机器也能打开。
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
