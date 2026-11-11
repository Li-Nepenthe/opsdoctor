// 本文件：order-service 接收靶场的 HTTP 下单请求。
// 从 orderservice.Run 读起，它装配好数据库、缓存、库存客户端和消息发布器，再开放订单接口。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Li-Nepenthe/opsdoctor/internal/shop/app/orderservice"
)

func main() {
	if err := orderservice.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "订单服务退出：", err)
		os.Exit(1)
	}
}
