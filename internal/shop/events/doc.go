// Package events 把已创建的订单送到 Kafka，由 order-worker 异步更新销量。订单号是分区键，消息头传递生产者的追踪上下文。
// 从 model.go 的消息契约读到 publisher.go，再看 consumer.go 的 process。消费者先让销量事务提交，再提交位点。两次提交之间若进程退出，消息会重投，sales 包按订单号去重。
//
//   - model.go：定义订单创建事件，校验编号、数量、金额和时间。
//   - publisher.go：发送订单事件，并等待 Kafka 确认。
//   - consumer.go：逐条处理消息，保留失败消息的位点。
//   - topic.go：在启动时创建或核对三个分区的订单主题。
package events
