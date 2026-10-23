// 本文件：解析库存命令并通过正式 gRPC 契约调用服务。
//
// parseStock 先拒绝非法输入，runStock 再建立客户端与 deadline。
// 它不直接改库存表，便于用户走完整请求链。
// 先读参数范围，尤其是 int64 转 int32 前的检查，再看 Deduct 为什么只调用一次。

package shopctl

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"time"

	inventoryv1 "github.com/Li-Nepenthe/opsdoctor/gen/inventory/v1"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// stockOptions 是已经通过本地校验的命令参数，不含数据库配置。
type stockOptions struct {
	operation string        // operation 只能是 get 或 deduct，暂不扩展计划外命令。
	skuID     int64         // skuID 对应 --sku，必须为正数。
	quantity  int32         // quantity 对应 --quantity；只在 deduct 中使用。
	timeout   time.Duration // timeout 对应 --timeout，覆盖本次 RPC 的连接与执行等待。
}

// parseStock 在建立客户端前拒绝错误参数，防止“敲错命令却已经改了数据”。
func parseStock(arguments []string, output io.Writer) (stockOptions, error) {
	if len(arguments) == 0 || (arguments[0] != "get" && arguments[0] != "deduct") {
		return stockOptions{}, fmt.Errorf("用法：stock get|deduct --sku ID [--quantity N] [--timeout 800ms]")
	}
	flags := flag.NewFlagSet("stock "+arguments[0], flag.ContinueOnError)
	flags.SetOutput(output)
	skuID := flags.Int64("sku", 0, "商品 ID，必须大于 0")
	quantity := flags.Int64("quantity", 1, "扣减件数，必须为正 int32")
	timeout := flags.Duration("timeout", 800*time.Millisecond, "本次调用的超时，包含连接等待")
	if err := flags.Parse(arguments[1:]); err != nil {
		return stockOptions{}, err
	}
	if flags.NArg() != 0 || *skuID <= 0 || *quantity <= 0 || *quantity > math.MaxInt32 || *timeout <= 0 {
		return stockOptions{}, fmt.Errorf("参数不合法：sku/quantity/timeout 必须为正，quantity 不得超过 int32，不能有多余位置参数")
	}
	// 先检查范围再转 int32；Go 的整数转换不会抛异常，直接转换过大的值会截断。
	return stockOptions{operation: arguments[0], skuID: *skuID, quantity: int32(*quantity), timeout: *timeout}, nil
}

// runStock 创建一个短生命周期客户端；本机靶场使用明文 HTTP/2，不能原样部署到公网。
func runStock(parent context.Context, arguments []string, reader config.Reader, output io.Writer) (result error) {
	options, err := parseStock(arguments, output)
	if err != nil {
		return err
	}
	target, err := reader.String("INVENTORY_GRPC_TARGET", "127.0.0.1:9001")
	if err != nil {
		return err
	}
	// NewClient 只组装连接管理器，不在这里阻塞连接；首次 RPC 的 context 也覆盖连接等待。
	connection, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry())
	if err != nil {
		return fmt.Errorf("创建库存客户端：%w", err)
	}
	defer func() { result = errors.Join(result, connection.Close()) }()
	ctx, cancel := context.WithTimeout(parent, options.timeout)
	defer cancel()
	client := inventoryv1.NewInventoryServiceClient(connection)
	if options.operation == "get" {
		stock, err := client.GetStock(ctx, &inventoryv1.GetStockRequest{SkuId: options.skuID})
		if err != nil {
			return fmt.Errorf("查询库存：%w", err)
		}
		_, err = fmt.Fprintf(output, "sku_id=%d available=%d\n", stock.GetSkuId(), stock.GetAvailable())
		return err
	}
	// Deduct 可能已经提交但响应丢失，不能自行循环重试；失败时将真实状态交给操作者判断。
	if _, err := client.Deduct(ctx, &inventoryv1.DeductRequest{SkuId: options.skuID, Quantity: options.quantity}); err != nil {
		return fmt.Errorf("扣减库存：%w", err)
	}
	_, err = fmt.Fprintf(output, "sku_id=%d deducted=%d\n", options.skuID, options.quantity)
	return err
}
