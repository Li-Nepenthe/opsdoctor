// 本文件：把订单创建消息发送到 Kafka。
//
// 订单已经写入后才调用这里。同步等待 Kafka 的答复，调用方才能知道这一步是否失败。
//
// 主要内容：
//   - Publisher、NewPublisher：配置按订单 ID 分区的同步生产者。
//   - Publish：校验消息，编码 JSON，并等待写入结果。
//   - Close：在服务停止接收新订单后关闭生产者。
//
// 阅读建议：Hash 使用消息 key 选择分区。这里用订单 ID，避免热门商品独占一个分区。

package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
)

// Publisher 使用 kafka.Writer 自带的连接复用，不为每个订单重新连接 broker。
type Publisher struct {
	writer    *kafka.Writer    // WriteMessages 会等待 broker 确认。
	transport *kafka.Transport // Close 时另外关闭空闲连接。
}

// 发送参数只约束 Kafka 客户端。订单请求自己的 context 仍能更早结束等待。
const (
	kafkaIOTimeout   = 3 * time.Second       // 与默认 HTTP 预算相当，连接或读写不能无限等待。
	kafkaIdleTimeout = 30 * time.Second      // 短时间内的订单复用连接，长时间空闲后再归还。
	publishAttempts  = 3                     // 消息有订单号去重，因此发送失败可以有限重试。
	publishBatchWait = 10 * time.Millisecond // 只等很短时间凑批，避免低流量下迟迟不返回。
)

// NewPublisher 禁用自动建主题。启动时由 EnsureTopic 统一检查分区数。
func NewPublisher(brokers []string) *Publisher {
	transport := &kafka.Transport{DialTimeout: kafkaIOTimeout, IdleTimeout: kafkaIdleTimeout}
	writer := &kafka.Writer{Addr: kafka.TCP(brokers...), Topic: OrderCreatedTopic, Balancer: &kafka.Hash{},
		RequiredAcks: kafka.RequireAll, MaxAttempts: publishAttempts, BatchTimeout: publishBatchWait,
		ReadTimeout: kafkaIOTimeout, WriteTimeout: kafkaIOTimeout, Transport: transport}
	return &Publisher{writer: writer, transport: transport}
}

// Publish 收到错误时不声称发送成功。即使响应丢失而重复投递，消费者也会按订单编号去重。
func (publisher *Publisher) Publish(ctx context.Context, event OrderCreated) error {
	if err := event.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("编码订单消息：%w", err)
	}
	// key 是十进制订单编号。生产者的 Hash 会把不同订单分散到三个分区。
	return publisher.writer.WriteMessages(ctx, kafka.Message{Key: []byte(strconv.FormatInt(event.OrderID, 10)), Value: data, Time: event.CreatedAt})
}

// Close 等发送中的调用结束后再执行，避免关闭过程与新的 Publish 争抢资源。
func (publisher *Publisher) Close() error {
	err := publisher.writer.Close()
	publisher.transport.CloseIdleConnections()
	return err
}
