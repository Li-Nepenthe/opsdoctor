// 本文件：chaos 在宿主机上运行故障实验。
// 从 internal/chaos/app 的 Run 读起，它把命令分派给相应场景，selftest 收到中断时会先尝试恢复现场。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/app"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := app.Run(ctx, os.Args[1:], config.New(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "chaos：", err)
		os.Exit(1)
	}
}
