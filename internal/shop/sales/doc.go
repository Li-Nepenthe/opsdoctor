// Package sales 处理 order-worker 收到的订单事件。去重凭据和销量增量在同一事务提交，数据库写完后即使消息再次送达，也不会重复计算销量。
// 从 service.go 的 Handle 读到 repository.go 的 Apply，再看 shopctl 调用的 Check 怎样在同一快照中核对两边的件数。
//
//   - model.go：保存订单总件数、已计入销量的件数及其差值。
//   - service.go：校验订单事件，将处理结果交还给消费者。
//   - repository.go：事务内登记订单号和销量，并提供只读的总量核对。
package sales
