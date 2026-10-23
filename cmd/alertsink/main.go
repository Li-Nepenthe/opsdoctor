// 告警接收器入口只负责退出码，具体装配由 platform/alertsink.Run 完成。
// 本目录只保留 main.go；学习通知鉴权请从 handler.go 开始。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/alertsink"
)

// main 等待清理完成后再退出，避免 os.Exit 跳过 HTTP 的关闭流程。
func main() {
	if err := alertsink.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "告警接收器退出：", err)
		os.Exit(1)
	}
}
