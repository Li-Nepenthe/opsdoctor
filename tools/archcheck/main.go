// 本文件：archcheck 的命令行入口负责选择检查目录，并用退出状态告知 CI 是否通过。
// 从 main 调用的 check 读起，文件布局、场景读取和部署规则在那里汇合。

package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	// 从模块根目录开始，兄弟目录也必须一起检查。
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
