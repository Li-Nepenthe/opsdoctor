// 本文件：分派注释检查、符号清单和文件地图三种命令模式。
//
// main 负责退出码，run 负责参数与输出。真正的扫描规则在 check.go/files.go。
// 先读模式互斥，再看即使用 -list 或 -map，也不会把缺注释当成检查成功。
//
// 主要内容：
//   - main：把命令行参数交给 run，并把失败转换为非零退出码，供 CI 阻止缺注释的提交。
//   - run：让检查、符号清单与文件地图共用一次扫描。传入 writer 后，测试可以直接检查输出而不用启动子进程。

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

// run 让检查、符号清单与文件地图共用一次扫描。传入 writer 后，测试可以直接检查输出而不用启动子进程。
func run(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("commentcheck", flag.ContinueOnError)
	flags.SetOutput(output)
	list := flags.Bool("list", false, "按包列出类型和函数的名称、位置、第一句注释")
	fileMap := flags.Bool("map", false, "按目录列出文件和说明，链接用于 docs/代码地图.md")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *list && *fileMap {
		return fmt.Errorf("-list 和 -map 不能同时使用，请分别生成符号清单与文件地图")
	}
	patterns := flags.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	// 先扫描再输出。即使使用 -list，缺注释也不会被当成检查通过。
	result, err := scan(patterns)
	if err != nil {
		return err
	}
	if *list {
		if err := writeList(output, result.symbols); err != nil {
			return err
		}
	}
	if *fileMap {
		if err := writeMap(output, result.sources); err != nil {
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
	if !*list && !*fileMap {
		_, err = fmt.Fprintf(output, "commentcheck：通过（%d 个非生成 Go 文件）\n", result.files)
	}
	return err
}
