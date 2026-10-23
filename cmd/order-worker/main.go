// 订单消费者入口只负责退出码，连接和停机顺序由 orderworker.Run 管理。
// 本目录只有 main.go，主要内容是 main；阅读消费过程请从 internal/shop/events 开始。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Li-Nepenthe/opsdoctor/internal/shop/app/orderworker"
)

// main 在 Run 返回并完成清理后才调用 os.Exit，不提前跳过数据库与 Kafka 的关闭。
func main() {
	if err := orderworker.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "订单消费者退出：", err)
		os.Exit(1)
	}
}
