// 本文件：分派宿主机调试命令并组织造数流程。
//
// main 把参数、环境读取器和输出传给 Run。每个子命令再去调用对应业务能力。
// 分派留在 app 中，cmd/main.go 就能保持简单。
//
// 主要内容：
//   - Run：分派 seed、stock 和 check，并处理停止信号。
//   - runSeed：连接数据库、执行迁移、批量造数，报告真实耗时。
//
// 阅读建议：先看命令分派，再读 runSeed。销量核对单独在 check.go。

package shopctl

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/lifecycle"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/app"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/migrations"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/seed"
)

// seedTimeout 给本机大批量写入十分钟预算。每条 SQL 还受 mysqlx 网络读写超时限制。
const seedTimeout = 10 * time.Minute

// Run 分派命令并处理 Ctrl+C。reader/output 显式注入，使参数测试不污染进程环境或标准输出。
func Run(parent context.Context, arguments []string, reader config.Reader, output io.Writer) error {
	if len(arguments) == 0 {
		return fmt.Errorf("用法：shopctl seed [--small] | stock get|deduct --sku ID [--quantity N] | check")
	}
	ctx, stop := lifecycle.Notify(parent)
	defer stop()
	switch arguments[0] {
	case "seed":
		return runSeed(ctx, arguments[1:], reader, output)
	case "stock":
		return runStock(ctx, arguments[1:], reader, output)
	case "check":
		return runCheck(ctx, arguments[1:], reader, output)
	default:
		return fmt.Errorf("不支持此命令；当前提供 seed、stock 和 check")
	}
}

// runSeed 一条命令完成连接、迁移和批量写入。计时从连接前开始，便于如实记录初始化总耗时。
func runSeed(parent context.Context, arguments []string, reader config.Reader, output io.Writer) (result error) {
	flags := flag.NewFlagSet("seed", flag.ContinueOnError)
	flags.SetOutput(output)
	small := flags.Bool("small", false, "仅生成 100 商品和 400 订单")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("seed 不接受位置参数")
	}
	settings, err := app.DatabaseConfig(reader)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, seedTimeout)
	defer cancel()
	started := time.Now()
	database, err := mysqlx.Open(ctx, settings)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, database.Close()) }()
	// 与服务共用嵌入迁移。因此先运行 seed 或先启动服务都能得到同一表结构。
	if err := migrations.Apply(ctx, database); err != nil {
		return err
	}
	generated, err := seed.Run(ctx, database, *small)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "products=%d stock=%d orders=%d inserted_products=%d skipped=%t seed_elapsed=%s total_elapsed=%s\n",
		generated.Products, generated.Products, generated.Orders, generated.InsertedProducts, generated.Skipped,
		generated.Duration.Round(time.Millisecond), time.Since(started).Round(time.Millisecond))
	return err
}
