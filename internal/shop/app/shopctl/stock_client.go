// 本文件：解析库存命令并通过正式 gRPC 契约调用服务。
//
// parseStock 先拒绝非法输入，runStock 再建立客户端与 deadline。
// 它不直接改库存表，便于用户走完整请求链。
// 先读参数范围，尤其是 int64 转 int32 前的检查，再看 Deduct 为什么只调用一次。
//
// 主要内容：
//   - parseStock：在建立客户端前拒绝错误参数，防止“敲错命令却已经改了数据”。
//   - runStock：创建一个短生命周期客户端，分派查询、扣减或补库存。
//   - restock：在实验前按明确范围调用 Release；遇到结果不确定立即停止。

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
	operation string        // get 查询，deduct 扣减，restock 通过正式 Release 接口补充实验库存。
	skuID     int64         // skuID 对应 --sku，必须为正数。
	quantity  int32         // quantity 对应 --quantity；每个商品增加或扣减的件数。
	count     int64         // --count 只用于 restock，表示从 sku 开始的连续商品数。
	timeout   time.Duration // timeout 对应 --timeout，覆盖本次 RPC 的连接与执行等待。
}

// parseStock 在建立客户端前拒绝错误参数，防止“敲错命令却已经改了数据”。
func parseStock(arguments []string, output io.Writer) (stockOptions, error) {
	if len(arguments) == 0 || (arguments[0] != "get" && arguments[0] != "deduct" && arguments[0] != "restock") {
		return stockOptions{}, fmt.Errorf("用法：stock get|deduct|restock --sku ID [--quantity N] [--count N] [--timeout 800ms]")
	}
	flags := flag.NewFlagSet("stock "+arguments[0], flag.ContinueOnError)
	flags.SetOutput(output)
	skuID := flags.Int64("sku", 0, "商品 ID，必须大于 0")
	quantity := flags.Int64("quantity", 1, "每个商品增加或扣减的件数，必须为正 int32")
	count := flags.Int64("count", 1, "补库存的连续商品数；仅 restock 可大于 1，最多 10000")
	timeout := flags.Duration("timeout", 800*time.Millisecond, "本次调用的超时，包含连接等待")
	if err := flags.Parse(arguments[1:]); err != nil {
		return stockOptions{}, err
	}
	if flags.NArg() != 0 || *skuID <= 0 || *quantity <= 0 || *quantity > math.MaxInt32 || *timeout <= 0 {
		return stockOptions{}, fmt.Errorf("参数不合法：sku/quantity/timeout 必须为正，quantity 不得超过 int32，不能有多余位置参数")
	}
	// 限制一次批量操作的范围。先检查上界再相加，避免最后一个商品编号发生整数溢出。
	const maximumRestockCount = 10000 // 一次覆盖实验常用商品；更大的范围由操作者分批明确执行。
	if *count < 1 || *count > maximumRestockCount || *skuID > math.MaxInt64-(*count-1) || (arguments[0] != "restock" && *count != 1) {
		return stockOptions{}, fmt.Errorf("count 必须为 1..10000，仅 restock 可批量操作，商品编号不得溢出")
	}
	// 先检查范围再转 int32。Go 的整数转换不会抛异常，直接转换过大的值会截断。
	return stockOptions{operation: arguments[0], skuID: *skuID, quantity: int32(*quantity), count: *count, timeout: *timeout}, nil
}

// runStock 创建一个短生命周期客户端。本机靶场使用明文 HTTP/2，不能原样部署到公网。
func runStock(parent context.Context, arguments []string, reader config.Reader, output io.Writer) (result error) {
	options, err := parseStock(arguments, output)
	if err != nil {
		return err
	}
	target, err := reader.String("INVENTORY_GRPC_TARGET", "127.0.0.1:9001")
	if err != nil {
		return err
	}
	// NewClient 只组装连接管理器，不在这里阻塞连接。首次 RPC 的 context 也覆盖连接等待。
	// 命令也可能使用 Compose 主机名。只查 IP，不等待本项目不使用的 DNS TXT 服务配置。
	connection, err := grpc.NewClient(target, grpc.WithDisableServiceConfig(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry())
	if err != nil {
		return fmt.Errorf("创建库存客户端：%w", err)
	}
	defer func() { result = errors.Join(result, connection.Close()) }()
	client := inventoryv1.NewInventoryServiceClient(connection)
	if options.operation == "restock" {
		return restock(parent, client, options, output)
	}
	ctx, cancel := context.WithTimeout(parent, options.timeout)
	defer cancel()
	if options.operation == "get" {
		stock, err := client.GetStock(ctx, &inventoryv1.GetStockRequest{SkuId: options.skuID})
		if err != nil {
			return fmt.Errorf("查询库存：%w", err)
		}
		_, err = fmt.Fprintf(output, "sku_id=%d available=%d\n", stock.GetSkuId(), stock.GetAvailable())
		return err
	}
	// Deduct 可能已经提交但响应丢失，不能自行循环重试。失败时将真实状态交给操作者判断。
	if _, err := client.Deduct(ctx, &inventoryv1.DeductRequest{SkuId: options.skuID, Quantity: options.quantity}); err != nil {
		return fmt.Errorf("扣减库存：%w", err)
	}
	_, err = fmt.Fprintf(output, "sku_id=%d deducted=%d\n", options.skuID, options.quantity)
	return err
}

// restock 在实验前逐个补库存，始终通过库存服务的校验和原子更新，不直接操作表。
// Release 没有请求去重：一旦未收到答复就停止，并报告失败的商品，操作者应先查询再决定是否继续。
func restock(parent context.Context, client inventoryv1.InventoryServiceClient, options stockOptions, output io.Writer) error {
	for offset := int64(0); offset < options.count; offset++ {
		skuID := options.skuID + offset
		// 每个 RPC 单独计时，前一批正常完成不会挤占后一批的执行时间；Ctrl+C 仍取消整个命令。
		ctx, cancel := context.WithTimeout(parent, options.timeout)
		_, err := client.Release(ctx, &inventoryv1.ReleaseRequest{SkuId: skuID, Quantity: options.quantity})
		cancel()
		if err != nil {
			return fmt.Errorf("补库存中止：前 %d 个商品已确认；sku=%d 结果需先查询，不能直接重跑整批：%w", offset, skuID, err)
		}
	}
	_, err := fmt.Fprintf(output, "restocked_from=%d restocked_through=%d added_each=%d confirmed=%d\n", options.skuID, options.skuID+options.count-1, options.quantity, options.count)
	return err
}
