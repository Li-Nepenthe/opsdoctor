// 本文件：定义订单创建事件的 JSON 契约。
//
// 订单服务把已写入的订单转换成这个值，消费者从同样的值读取销量变更。
// 单独共享消息模型，双方就不必互相导入订单或销量的具体实现。
//
// 主要内容：
//   - OrderCreated：承载计划书约定的六个字段。
//   - Validate：拒绝缺字段、非正数量和不合法时间。
//
// 阅读建议：金额仍以分保存，时间固定为 UTC。不要用字符串解析来猜订单编号。

package events

import (
	"fmt"
	"time"
)

// OrderCreated 对应 order.created 的消息体，去重依据是 OrderID。
type OrderCreated struct {
	OrderID     int64     `json:"order_id"`     // 同一订单重发时保持这个编号不变。
	UserID      int64     `json:"user_id"`      // 当前靶场直接接收请求中的用户编号。
	SKUID       int64     `json:"sku_id"`       // 商品只有一个 SKU，所以它等于商品 ID。
	Quantity    int32     `json:"quantity"`     // 销量按购买件数累加，不是按订单笔数。
	AmountCents int64     `json:"amount_cents"` // 金额使用整数，避免浮点误差。
	CreatedAt   time.Time `json:"created_at"`   // JSON 编码使用 RFC 3339，发送端统一转 UTC。
}

// Validate 在写入销量之前挡住损坏消息，避免错误数据污染汇总表。
func (event OrderCreated) Validate() error {
	_, offset := event.CreatedAt.Zone()
	if event.OrderID <= 0 || event.UserID <= 0 || event.SKUID <= 0 || event.Quantity <= 0 || event.AmountCents < 0 || event.CreatedAt.IsZero() || offset != 0 {
		return fmt.Errorf("订单创建消息字段不合法")
	}
	return nil
}
