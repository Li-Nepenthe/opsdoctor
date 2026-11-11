// 本文件：定义商品与详情响应，协议须能区分已知库存为零和库存暂不可用。
// 从 StockView 可以看到这两种情况在详情接口中的表示。

package product

// Product 的价格使用整数分，避免浮点计算把金额算出小数误差。
type Product struct {
	ID         int64  `json:"id"` // ID 与库存中的 sku_id 相同。
	Name       string `json:"name"`
	PriceCents int64  `json:"price_cents"`
}

// StockView 的库存数值只在查询成功时出现，售罄时仍返回零。
type StockView struct {
	Status    string `json:"status"` // 只有 ok 和 unavailable 两种公开状态。
	Available *int32 `json:"available,omitempty"`
}

// Detail 的商品字段在响应顶层，库存单独放在 stock 中。
type Detail struct {
	Product
	Stock StockView `json:"stock"`
}

// Error 标识商品查询中可对外区分的业务结果。
type Error string

// Error 返回固定的业务错误描述。
func (err Error) Error() string { return string(err) }

// 这两类错误不代表数据库故障，HTTP 层分别返回 400 和 404。
const (
	ErrInvalidArgument Error = "商品编号不合法"
	ErrNotFound        Error = "商品不存在"
)
