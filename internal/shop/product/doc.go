// Package product 供订单服务查商品价格和展示详情。它采用缓存旁路（Cache Aside）：先读 Redis，未命中再查 MySQL 并回填。库存不可用时仍能展示商品，下单则须另外扣减库存。
// 从 service.go 的 Get 和 Detail 入手，再读 cache.go 的空值与过期规则。
//
//   - model.go：定义商品及详情响应，并区分未知库存和零库存。
//   - service.go：合并同商品的并发回源，并在库存查询失败时保留商品详情。
//   - repository.go：按主键读取商品名称和价格。
//   - cache.go：读写商品和空值缓存，打散正常商品的过期时间。
//   - http_handler.go：接收商品详情请求并返回对应的 HTTP 状态。
package product
