// 本文件：验证批量补库存遇到结果不确定时会立即停止。
//
// 同一条 Release 重发可能把库存再加一次。这里用假客户端记录调用，
// 让第二个商品返回超时，检查命令既不会重试它，也不会继续处理后面的商品。
//
// 主要内容：
//   - restockClient.Release：记录收到的商品编号，并模拟一次超时。
//   - TestRestockStopsOnUncertainty：核对部分成功后的停止位置与提示。
//   - TestRestockRange：核对正常批次只处理明确指定的连续范围。
//
// 阅读建议：先看失败用例，理解为什么这里不能自动重跑整批。

package shopctl

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	inventoryv1 "github.com/Li-Nepenthe/opsdoctor/gen/inventory/v1"
	"google.golang.org/grpc"
)

// restockClient 只替换命令真正会用到的 Release，其余方法由嵌入接口补齐类型要求。
type restockClient struct {
	inventoryv1.InventoryServiceClient         // 测试没有查询或扣减调用；误调用会立刻暴露。
	calls                              []int64 // restock 顺序调用，测试返回后读取，不需要并发锁。
	failAt                             int64   // 此商品返回超时，0 表示全部成功。
}

// Release 模拟“答复没回来”，不宣称库存服务一定没有处理。
func (client *restockClient) Release(ctx context.Context, request *inventoryv1.ReleaseRequest, _ ...grpc.CallOption) (*inventoryv1.ReleaseResponse, error) {
	client.calls = append(client.calls, request.SkuId)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.SkuId == client.failAt {
		return nil, context.DeadlineExceeded
	}
	return &inventoryv1.ReleaseResponse{}, nil
}

// TestRestockStopsOnUncertainty 保证操作者能够知道哪部分已确认，哪一件还需要查询。
func TestRestockStopsOnUncertainty(t *testing.T) {
	client := &restockClient{failAt: 11}
	var output bytes.Buffer
	err := restock(t.Context(), client, stockOptions{skuID: 10, count: 3, quantity: 100, timeout: time.Second}, &output)
	if err == nil || !strings.Contains(err.Error(), "前 1 个商品已确认；sku=11") {
		t.Fatalf("没有指出待核对位置：%v", err)
	}
	if !slices.Equal(client.calls, []int64{10, 11}) || output.Len() != 0 {
		t.Fatalf("调用=%v 输出=%q", client.calls, output.String())
	}
}

// TestRestockRange 证明批量参数没有让命令越过操作者指定的最后一个商品。
func TestRestockRange(t *testing.T) {
	client := &restockClient{}
	var output bytes.Buffer
	err := restock(t.Context(), client, stockOptions{skuID: 10, count: 3, quantity: 100, timeout: time.Second}, &output)
	if err != nil || !slices.Equal(client.calls, []int64{10, 11, 12}) || !strings.Contains(output.String(), "confirmed=3") {
		t.Fatalf("调用=%v 输出=%q 错误=%v", client.calls, output.String(), err)
	}
}
