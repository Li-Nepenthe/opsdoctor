// Package order 负责下单和查订单，以及防止同一个下单请求被执行两次（接口幂等）。
//
// 一次下单的步骤（service.go 的 Create）：
//  1. 占位：用请求头里的 Idempotency-Key 在 Redis 里登记“这个请求我正在处理”。
//     同一个键的重复请求会看到这条记录，不会再执行一遍。
//  2. 查价格（通过 product 包）。
//  3. 扣库存（调用库存服务）。
//  4. 在 MySQL 里写订单。
//  5. 发一条“订单已创建”的 Kafka 消息，order-worker 收到后去加销量。
//  6. 把下单结果存回 Redis。同一个键再来请求，直接返回这个结果。
//
// 难点在于失败处理。这几步分布在 Redis、库存服务、MySQL、Kafka 四个地方，没有一个
// 能把它们包起来的事务。所以每一步失败时，都要先判断“到目前为止，哪些事已经真的发生了”：
//   - 确认什么都没发生（或者已经撤销干净）：删掉占位，客户端可以重试。
//   - 不确定有没有发生（比如扣库存超时）：保留占位，标记为“结果待核对”，记告警日志，等人工检查。
//     这时绝不能让客户端重试，否则可能扣两次库存、下两笔订单。
//
// 谁调用它：订单服务的 HTTP 接口。价格查询、库存扣减、发消息这三种能力由 orderservice 传进来，
// 本包只依赖它们的接口，测试时可以全部换成假的。
//
// 文件清单：
//   - doc.go：本说明。
//   - model.go：订单、下单请求、分页结果和错误类型。
//   - idempotency.go：幂等记录的三种状态（处理中、已完成、待核对），以及存取这些记录的接口。
//   - service.go：下单的六个步骤，以及每一步失败时该怎么办。
//   - repository.go：写订单、查订单详情、按用户分页查订单的 SQL。
//   - cache.go：幂等记录在 Redis 里的实现：占位、保存结果、标记待核对、删除占位。
//   - http_handler.go：下单和查订单的 HTTP 接口。
//   - service_test.go：用假的依赖，把每一种失败情况都走一遍。
//   - cache_integration_test.go：连真实 Redis，测试多个请求同时抢占位时只有一个成功。
//   - repository_integration_test.go：连真实 MySQL，测试写订单和分页查询。
//   - http_handler_test.go：测试请求格式、状态码和响应格式。
//
// 阅读顺序：model.go → idempotency.go → service.go 的 Create（对照上面六步）→ cache.go。
package order
