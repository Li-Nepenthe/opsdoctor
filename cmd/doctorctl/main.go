// 本文件：医生手动取证命令的入口。
//
// 主要内容：main 把命令行交给 app.RunCLI，并在失败时返回非零退出码。
// 阅读建议：先看 internal/doctor/app/cli.go，再看 HTTP 层如何进入同一工具注册表。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/app"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// main 不接收密码参数，登录令牌和医生地址都由环境变量提供。
func main() {
	if err := app.RunCLI(context.Background(), os.Args[1:], config.New(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "doctorctl：", err)
		os.Exit(1)
	}
}
