// 库存服务入口保持很薄；组装、信号处理与资源回收都由 inventoryservice 管理。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Li-Nepenthe/opsdoctor/internal/shop/app/inventoryservice"
)

// main 只把运行结果转成进程退出码；Run 返回前已执行 defer，os.Exit 不会绕过它的清理。
func main() {
	if err := inventoryservice.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "库存服务退出：", err)
		os.Exit(1)
	}
}
