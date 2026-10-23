// Package order 负责下单、订单查询和请求去重。
// 下单先占用幂等键，再查价格、扣库存、写订单和发消息。只在确认没有留下变更时放弃占位。
// 结果不确定时保留记录，交给人工核对。本包不假装跨服务操作是一个完整事务。
// orderservice 调用本包。MySQL、Redis 实现放在本包内，价格、库存和发消息能力由 app 注入。
//
// 文件清单：
//   - doc.go：说明订单流程与阅读顺序。
//   - model.go：订单、请求、分页和错误类型。
//   - service.go：下单顺序、补偿与结果待核对的判断。
//   - repository.go：订单写入事务、详情查询和按用户分页。
//   - idempotency.go：幂等记录的状态与业务所需接口。
//   - cache.go：Redis 占位、结果回放与按所有者更新记录。
//   - http_handler.go：验证 HTTP 参数并返回约定错误码。
//   - service_test.go：用内存替身验证失败边界与相同请求回放。
//   - cache_integration_test.go：在专用 Redis 数据库验证占位竞争和所有权。
//   - repository_integration_test.go：在专用 MySQL 库验证订单写入与分页。
//   - http_handler_test.go：验证请求格式、状态码与响应契约。
//
// 建议先读 model.go 和 idempotency.go，再沿 service.go 的 Create 往下读。
package order
