// 本文件：eval 在宿主机上调度评测，也可以导出已经保存的结果。
// 从 runner.RunCLI 读起，按 Ctrl+C 可以中断等待，已保存的诊断仍可查阅。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/Li-Nepenthe/opsdoctor/internal/eval/runner"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := runner.RunCLI(ctx, os.Args[1:], config.New(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "eval：", err)
		os.Exit(1)
	}
}
