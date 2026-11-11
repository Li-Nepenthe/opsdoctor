// 本文件：doctor 启动接收告警、调度诊断的医生服务。
// 从 internal/doctor/app 的 Run 读起，它先准备医生数据库和接入配置，再开放 HTTP 端口。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/app"
)

func main() {
	if err := app.Run(context.Background(), os.Args[1:]...); err != nil {
		fmt.Fprintln(os.Stderr, "医生服务退出：", err)
		os.Exit(1)
	}
}
