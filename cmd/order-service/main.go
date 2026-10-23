// 订单服务入口只转换退出码；资源装配、信号处理和请求排空由 orderservice.Run 完成。
// 本目录只有 main.go，主要内容是 main，阅读业务请从 internal/shop/app/orderservice 开始。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Li-Nepenthe/opsdoctor/internal/shop/app/orderservice"
)

// main 等 Run 清理资源后再退出，避免 os.Exit 绕过仍未执行的关闭动作。
func main() {
	if err := orderservice.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "订单服务退出：", err)
		os.Exit(1)
	}
}
