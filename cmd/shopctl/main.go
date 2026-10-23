// shopctl 是宿主机调试命令入口；实际参数解析和资源管理在 app/shopctl 中完成。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/app/shopctl"
)

// main 注入进程参数、环境读取器和输出；失败后用非零退出码让脚本或 CI 停止后续步骤。
func main() {
	if err := shopctl.Run(context.Background(), os.Args[1:], config.New(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "shopctl：", err)
		os.Exit(1)
	}
}
