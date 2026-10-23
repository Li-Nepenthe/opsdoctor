package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

// main 把命令行参数交给 run，并把失败转换为非零退出码，供 CI 阻止缺注释的提交。
func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run 解析 -list 与目录参数；检查错误或文件读取错误都会返回，输出写入调用方提供的 writer。
// 这个入口便于测试命令行为，无需启动子进程；扫描和写入均为同步操作。
func run(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("commentcheck", flag.ContinueOnError)
	flags.SetOutput(output)
	list := flags.Bool("list", false, "按包列出类型和函数的名称、位置、第一句注释")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	patterns := flags.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	// 先扫描再输出；即使使用 -list，缺注释也不会被当成检查通过。
	result, err := scan(patterns)
	if err != nil {
		return err
	}
	if *list {
		if err := writeList(output, result.symbols); err != nil {
			return err
		}
	}
	for _, problem := range result.problems {
		if _, err := fmt.Fprintln(output, problem); err != nil {
			return fmt.Errorf("输出检查结果：%w", err)
		}
	}
	if len(result.problems) != 0 {
		return fmt.Errorf("commentcheck：发现 %d 项问题", len(result.problems))
	}
	if !*list {
		_, err = fmt.Fprintf(output, "commentcheck：通过（%d 个非生成 Go 文件）\n", result.files)
	}
	return err
}
