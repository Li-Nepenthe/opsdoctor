// 本文件：运行注释检查，按命令行选项生成阅读清单或代码地图。
// 从 run 看三种模式怎样共用一次扫描。发现违规时先输出位置，再向 CI 返回失败。

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

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
