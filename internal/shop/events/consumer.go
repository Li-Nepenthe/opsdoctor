// 本文件：逐条处理 Kafka 消息，并在数据库成功后提交读取位置。
//
// “位点”就是某个分区读到了哪里。提交得太早，进程崩溃后就会跳过还没处理的消息。
// 本文件负责消费顺序，销量是否重复累加则由 sales 的数据库事务负责。
//
// 主要内容：
//   - MessageReader：保留拉取、提交两个动作，方便测试它们的顺序。
//   - NewReader：配置 order-worker 消费组和手动提交。
//   - Consumer、NewConsumer：连接读取器与消息处理函数。
//   - Run：收到停止信号后不再取新消息，完成手上这条消息再返回。
//   - process：严格解析事件，处理成功后才提交位点。
//
// 阅读建议：对照 FetchMessage 和 CommitMessages，理解为什么它们之间不能省略数据库事务。

package events

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
)

// MessageReader 是消费者需要的最小能力，真实实现是 kafka.Reader。
type MessageReader interface {
	FetchMessage(context.Context) (kafka.Message, error)
	CommitMessages(context.Context, ...kafka.Message) error
}

// 拉取限额控制等待与内存，不改变提交位点的时机。
const (
	consumeMaxBytes = 1 << 20                // 正常事件只有几个字段，一 MiB 足以容纳一批消息。
	consumeMaxWait  = 500 * time.Millisecond // 低流量下最多等半秒，不为凑批长时间等待。
)

// Consumer 不拥有 reader 的关闭权，app 等 Run 返回后统一关闭连接。
type Consumer struct {
	reader  MessageReader                             // Fetch 和 Commit 分开，提交时机由这里决定。
	handle  func(context.Context, OrderCreated) error // 数据库成功提交后才返回 nil。
	timeout time.Duration                             // 处理一条消息与提交位点共享这份预算。
	logger  *slog.Logger                              // 记录位置与订单编号，不输出完整消息体。
}

// NewReader 从消费组已提交的位置继续读取，新组则从最早保留的消息开始。
func NewReader(brokers []string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, Topic: OrderCreatedTopic, GroupID: "order-worker",
		StartOffset: kafka.FirstOffset, CommitInterval: 0, MinBytes: 1, MaxBytes: consumeMaxBytes,
		MaxWait: consumeMaxWait, WatchPartitionChanges: true})
}

// NewConsumer 要求有限的处理预算，停止服务时不能无限等一条消息。
func NewConsumer(reader MessageReader, handle func(context.Context, OrderCreated) error, timeout time.Duration, logger *slog.Logger) (*Consumer, error) {
	if reader == nil || handle == nil || timeout <= 0 {
		return nil, fmt.Errorf("消息消费者需要读取器、处理函数和正超时")
	}
	return &Consumer{reader: reader, handle: handle, timeout: timeout, logger: logger}, nil
}

// Run 串行处理消息。同一分区后面的消息不会越过前面尚未成功的消息提交。
func (consumer *Consumer) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		message, err := consumer.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("读取订单消息：%w", err)
		}
		// WithoutCancel 保留日志等上下文值，但不继承停止信号的取消。
		// 随后立即加自己的超时，让当前消息有机会做完，同时保证停机有上限。
		processing, cancel := context.WithTimeout(context.WithoutCancel(ctx), consumer.timeout)
		err = consumer.process(processing, message)
		cancel()
		if err != nil {
			consumer.logger.Error("订单消息处理未完成", "partition", message.Partition, "offset", message.Offset)
			// 不跳过坏消息。进程退出后由运维检查，重启仍从未提交的位置继续。
			return err
		}
	}
}

// process 把解析、处理、提交写在同一处，读者可以直接核对先后顺序。
func (consumer *Consumer) process(ctx context.Context, message kafka.Message) error {
	var event OrderCreated
	decoder := json.NewDecoder(bytes.NewReader(message.Value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return fmt.Errorf("订单消息 JSON 不合法")
	}
	// 一条 Kafka 消息只能承载一个 JSON 值，尾随第二个对象也应拒绝。
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("订单消息存在多余内容")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	if string(message.Key) != strconv.FormatInt(event.OrderID, 10) {
		return fmt.Errorf("订单消息 key 与订单编号不一致")
	}
	if err := consumer.handle(ctx, event); err != nil {
		return fmt.Errorf("更新销量：%w", err)
	}
	if err := consumer.reader.CommitMessages(ctx, message); err != nil {
		return fmt.Errorf("提交订单消息位点：%w", err)
	}
	return nil
}
