// 本文件：order-worker 消费订单消息，更新商品销量。
// 从 orderworker.Run 读起，它先停消费再关连接，给手中的消息留下收尾时间。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Li-Nepenthe/opsdoctor/internal/shop/app/orderworker"
)

func main() {
	if err := orderworker.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "订单消费者退出：", err)
		os.Exit(1)
	}
}
