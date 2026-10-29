// eval 是宿主机评测入口；实验配置、任务保存和结果导出在 internal/eval 中实现。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/Li-Nepenthe/opsdoctor/internal/eval/runner"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// main 把 Ctrl+C 传给等待任务的流程，已经创建的诊断和结果不会因终端退出被删除。
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := runner.RunCLI(ctx, os.Args[1:], config.New(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "eval：", err)
		os.Exit(1)
	}
}
