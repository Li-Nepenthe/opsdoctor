// 本文件：定义商品查询使用的数据和错误。
//
// service 负责填充这些值，HTTP 层负责把它们编码成 JSON。
// 单独定义模型，可以让缓存和 MySQL 使用同一种商品表示。
//
// 主要内容：
//   - Product：商品编号、名字和以分为单位的价格。
//   - StockView、Detail：商品详情展示的库存状态。
//   - Error：参数不合法和商品不存在这两类业务错误。
//
// 阅读建议：先看 Available 为什么是指针，再看 JSON 的 omitempty。

package product

// Product 的价格使用整数分，避免浮点计算把金额算出小数误差。
type Product struct {
	ID         int64  `json:"id"`          // ID 与库存中的 sku_id 相同。
	Name       string `json:"name"`        // Name 是展示名称，不参与库存判断。
	PriceCents int64  `json:"price_cents"` // PriceCents 是一件商品的单价。
}

// StockView 用指针区分“库存是零”和“没有查到库存数值”。
type StockView struct {
	Status    string `json:"status"`              // 只有 ok 和 unavailable 两种公开状态。
	Available *int32 `json:"available,omitempty"` // nil 配合 omitempty 会省略字段；指向 0 时仍输出 0。
}

// Detail 通过匿名嵌入 Product 复用商品字段，JSON 编码时这些字段仍位于最外层。
type Detail struct {
	Product           // 嵌入字段不用再写一层 product 对象，符合接口契约。
	Stock   StockView `json:"stock"` // 库存查不到时也保留状态说明。
}

// Error 是可比较的字符串类型，业务层用 errors.Is 判断它。
type Error string

// Error 让具名字符串满足 Go 的 error 接口。
func (err Error) Error() string { return string(err) }

// 这两类错误不代表数据库故障，HTTP 层分别返回 400 和 404。
const (
	ErrInvalidArgument Error = "商品编号不合法"
	ErrNotFound        Error = "商品不存在"
)
