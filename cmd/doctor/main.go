// 本文件：医生服务入口，只负责把启动结果变成进程退出码。
//
// 主要内容：main 调用 app.Run，等待资源清理后再退出。
// 阅读建议：具体装配从 internal/doctor/app 开始读。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/app"
)

// main 把 os.Exit 放在 Run 返回之后，避免跳过数据库和 HTTP 的清理。
func main() {
	if err := app.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "医生服务退出：", err)
		os.Exit(1)
	}
}
