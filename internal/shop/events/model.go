// 本文件：生产者与消费者共用订单事件契约，以订单号去重、按商品件数累加销量。
// 发送端在编码前、消费端在解码后调用 Validate，避免无效消息写入销量。

package events

import (
	"fmt"
	"time"
)

// OrderCreated 对应 order.created 的消息体，去重依据是 OrderID。
type OrderCreated struct {
	OrderID     int64     `json:"order_id"` // 同一订单重发时保持这个编号不变。
	UserID      int64     `json:"user_id"`
	SKUID       int64     `json:"sku_id"`   // 商品只有一个 SKU，所以它等于商品 ID。
	Quantity    int32     `json:"quantity"` // 销量按购买件数累加，不是按订单笔数。
	AmountCents int64     `json:"amount_cents"`
	CreatedAt   time.Time `json:"created_at"` // 发送端统一转为 UTC。
}

// Validate 在写入销量之前挡住损坏消息，避免错误数据污染汇总表。
func (event OrderCreated) Validate() error {
	_, offset := event.CreatedAt.Zone()
	if event.OrderID <= 0 || event.UserID <= 0 || event.SKUID <= 0 || event.Quantity <= 0 || event.AmountCents < 0 || event.CreatedAt.IsZero() || offset != 0 {
		return fmt.Errorf("订单创建消息字段不合法")
	}
	return nil
}
