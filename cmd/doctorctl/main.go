// 本文件：doctorctl 通过医生的 HTTP 接口查看系统和运行工具。
// 从 internal/doctor/app 的 RunCLI 读起，医生地址和登录令牌由环境提供。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/app"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

func main() {
	if err := app.RunCLI(context.Background(), os.Args[1:], config.New(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "doctorctl：", err)
		os.Exit(1)
	}
}
