// 本文件：销量核对比较同一快照中的订单件数和销量；消费者尚未追平时也可能出现差值。
// shopctl 使用 Summary 展示核对结果，Gap 的符号保留差额方向。

package sales

// Summary 保存 shopctl check 核对的件数和差值。
type Summary struct {
	Ordered int64 `json:"ordered_quantity"`
	Sold    int64 `json:"sold_quantity"`
	Gap     int64 `json:"gap"` // 正值可能是消费延迟或消息发送失败；负值需要排查重复统计或人工改动。
}
