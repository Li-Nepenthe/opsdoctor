// chaos 是宿主机实验命令入口；场景、执行与验收在 internal/chaos 中组织。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/app"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// main 将 Ctrl+C 变成取消信号，selftest 收到后先恢复当前场景再退出。
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := app.Run(ctx, os.Args[1:], config.New(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "chaos：", err)
		os.Exit(1)
	}
}
