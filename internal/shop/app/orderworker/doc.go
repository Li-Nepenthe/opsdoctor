// Package orderworker 把 Kafka 订单消息交给销量服务。停机时先停止取新消息，让手中的消息在自己的期限内写入销量并提交位点，随后关闭连接。
//
// config.go：读取管理端口、Kafka 地址和单条消息处理期限，并在联网前校验。
// app.go：启动消费循环与管理端口，等两侧退出后归还 Kafka 和数据库连接。
package orderworker
