// 本文件：loadgen 为订单服务生成压测流量。
// 从 loadgen.Run 读起，停止时它等待已经发出的请求结束，再输出本次统计。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Li-Nepenthe/opsdoctor/internal/loadgen"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

func main() {
	if err := loadgen.Run(context.Background(), os.Args[1:], config.New(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "压测工具退出：", err)
		os.Exit(1)
	}
}
