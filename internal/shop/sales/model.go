// 本文件：表示订单数量与已统计销量的核对结果。
//
// shopctl check 展示这里的值。核对单位是商品件数，不能拿订单笔数和销量比较。
//
// 主要内容：
//   - Summary：保存两个总量，以及“订单件数减已统计销量”的差值。
//
// 阅读建议：差值非零只说明还没有对齐，不能单靠它断言消息丢了。

package sales

// Summary 是一次数据库快照里的总量。消费尚未追平时，Gap 可能暂时大于零。
type Summary struct {
	Ordered int64 `json:"ordered_quantity"` // 所有订单购买件数之和。
	Sold    int64 `json:"sold_quantity"`    // product_sales 已统计的件数之和。
	Gap     int64 `json:"gap"`              // 正值可能是消费延迟或消息发送失败；负值需要排查重复统计或人工改动。
}
