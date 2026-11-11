// 本文件：alertsink 接收 Alertmanager 的通知，供靶场实验查看告警摘要。
// 从 alertsink.Run 读起，接收器先检查令牌配置，再开始监听。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/alertsink"
)

func main() {
	if err := alertsink.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "告警接收器退出：", err)
		os.Exit(1)
	}
}
