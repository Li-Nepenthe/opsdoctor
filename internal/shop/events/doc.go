// Package events 定义订单创建消息，并负责它在 Kafka 中的发送与接收。
// order 只需要 Publisher，销量流程只需要处理一条 OrderCreated。Kafka 连接由 app 管理。
// 当前阶段没有接入调用链 SDK。后续阶段在本包补入约定的 traceparent 传播。
// 传输依赖 kafka-go。本包不直接访问 MySQL，实际记账通过处理函数交给 sales。
//
// 文件清单：
//   - doc.go：说明消息边界与阅读顺序。
//   - model.go：订单创建消息的字段和校验。
//   - publisher.go：按订单 ID 分区并同步发送消息。
//   - consumer.go：先处理数据库，再手动提交 Kafka 位点。
//   - topic.go：创建或检查三分区主题。
//   - consumer_test.go：检查提交顺序、重复投递和停止时的处理边界。
//
// 先读 model.go 的契约，再看 consumer.go 为什么没有使用自动提交。
package events
