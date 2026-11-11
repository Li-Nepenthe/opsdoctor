// 本文件：inventory-service 提供库存查询和扣减服务。
// 从 inventoryservice.Run 读起，它在库存服务和管理端口都准备好后才接收请求。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Li-Nepenthe/opsdoctor/internal/shop/app/inventoryservice"
)

func main() {
	if err := inventoryservice.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "库存服务退出：", err)
		os.Exit(1)
	}
}
