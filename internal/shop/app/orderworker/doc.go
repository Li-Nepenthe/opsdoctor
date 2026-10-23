// Package orderworker 组装订单消息消费者进程。
// 停止信号先阻止继续拉取消息，当前消息仍有有限时间完成数据库事务和位点提交。
// cmd/order-worker 调用 Run。本包依赖 events 的消费循环和 sales 的 MySQL 实现，不编写记账 SQL。
//
// 文件清单：
//   - doc.go：说明消费者进程的资源边界。
//   - app.go：读取配置，装配 Kafka 与销量仓储，启动管理端口并处理退出。
//
// 先看 Run 建立了哪些资源，再看 serve 为什么等消费循环返回后才关闭 reader。
package orderworker
