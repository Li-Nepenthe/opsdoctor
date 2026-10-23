// 压测入口只传递参数、环境读取器与输出；调度和控制端口由 loadgen.Run 管理。
// 本目录只有 main.go，主要内容是 main；阅读发送、重试和统计请从 internal/loadgen 开始。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Li-Nepenthe/opsdoctor/internal/loadgen"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// main 等待在途请求排空与最终报告输出后再决定退出码。
func main() {
	if err := loadgen.Run(context.Background(), os.Args[1:], config.New(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "压测工具退出：", err)
		os.Exit(1)
	}
}
