// 本文件：在内存中核对造数行数、编号和占位符。
//
// TestMakeBatches 使用两个商品的小样本，便于手算四单关系。先看订单编号断言，再看参数数量是否与问号数量一致。数据库事务的原子性不能靠这个测试证明。
//
// 主要内容：
//   - TestMakeBatches：验证两个 SKU 的五表基线互相对应。此测试只生成内存参数，不连接数据库。

package seed

import (
	"strings"
	"testing"
	"time"
)

// TestMakeBatches 验证两个 SKU 的五表基线互相对应。此测试只生成内存参数，不连接数据库。
func TestMakeBatches(t *testing.T) {
	now := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	batches := makeBatches(2, 3, now)
	counts := []int{2, 2, 8, 2, 8} // 商品、库存、订单、销量、去重记录分别应有这些行数。
	for i, data := range batches {
		if len(data.rows) != counts[i] {
			t.Fatalf("表 %d 行数：%d", i, len(data.rows))
		}
		statement, arguments := insertStatement(data)
		if strings.Count(statement, "?") != len(arguments) {
			t.Fatalf("表 %d 占位符和参数不匹配", i)
		}
	}
	// SKU=2 的四单编号为 5..8。单价和总价都是整数分，销量记录恰好是四件。
	if batches[2].rows[0][0] != int64(5) || batches[2].rows[7][0] != int64(12) || batches[3].rows[0][1] != ordersPerProduct {
		t.Fatal("订单编号或销量基线不匹配")
	}
	if strings.Contains(batches[0].statement, "商品") {
		t.Fatal("业务数据被拼进 SQL")
	}
}
