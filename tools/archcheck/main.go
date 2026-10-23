package main

import (
	"fmt"
	"os"
	"strings"
)

// main 检查指定模块根目录；默认 ./...，违规时返回非零退出码供 CI 使用。
func main() {
	// 命令接受模块根目录而不是任意 Go 包表达式，这样结构检查不会漏掉兄弟目录。
	root := "."
	if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "用法：archcheck [模块根目录/...]")
		os.Exit(1)
	}
	if len(os.Args) == 2 {
		root = strings.TrimSuffix(os.Args[1], "/...")
	}
	problems, err := check(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, problem := range problems {
		fmt.Fprintln(os.Stderr, problem)
	}
	if len(problems) != 0 {
		fmt.Fprintf(os.Stderr, "archcheck：发现 %d 项问题\n", len(problems))
		os.Exit(1)
	}
	fmt.Println("archcheck：通过")
}
