// Package product 负责商品价格查询和商品详情。
// 详情会附带库存，但库存暂时查不到时仍返回商品信息。下单只借用这里的价格查询。
// orderservice 和 order 调用本包。SQL 依赖 MySQL，缓存依赖 Redis，库存读取通过小接口注入。
// 本包不扣库存，也不写订单。库存展示不能代替下单时的原子扣减。
//
// 文件清单：
//   - doc.go：说明商品查询的边界和阅读顺序。
//   - model.go：商品、库存展示和错误类型。
//   - service.go：先查缓存，再查数据库。合并同一商品的同时回源请求。
//   - repository.go：按主键从 MySQL 读取商品。
//   - cache.go：读写 Redis 商品缓存，设置空值缓存和随机过期时间。
//   - http_handler.go：处理商品 ID，并按 HTTP 契约返回详情。
//   - http_handler_test.go：区分商品查询超时与库存展示降级，核对统一错误码。
//   - service_test.go：验证缓存路径、合并请求和库存降级。
//   - cache_test.go：验证缓存内容与过期时间的计算。
//   - cache_integration_test.go：在 Redis 专用数据库验证内容与真实 TTL。
//
// 建议按 model.go、service.go、repository.go、cache.go 的顺序阅读，最后看 HTTP 入口。
package product
