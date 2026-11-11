// 本文件：shopctl 在宿主机上造数、操作库存和核对销量。
// 从 shopctl.Run 读起，各子命令只建立自己需要的连接。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/app/shopctl"
)

func main() {
	if err := shopctl.Run(context.Background(), os.Args[1:], config.New(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "shopctl：", err)
		os.Exit(1)
	}
}
