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
//   - decodeMessage：在业务事务前检查消息结构、字段和订单 key。
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

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	reader    MessageReader                                     // Fetch 和 Commit 分开，提交时机由这里决定。
	handle    func(context.Context, OrderCreated) (bool, error) // bool 表示重复；无错误才允许提交位点。
	timeout   time.Duration                                     // 处理一条消息与提交位点共享这份预算。
	logger    *slog.Logger                                      // 记录位置与订单编号，不输出完整消息体。
	telemetry *telemetry.Runtime                                // span 和指标属于进程，消费者不负责关闭它们。
}

// NewReader 从消费组已提交的位置继续读取，新组则从最早保留的消息开始。
func NewReader(brokers []string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, Topic: OrderCreatedTopic, GroupID: "order-worker",
		StartOffset: kafka.FirstOffset, CommitInterval: 0, MinBytes: 1, MaxBytes: consumeMaxBytes,
		MaxWait: consumeMaxWait, WatchPartitionChanges: true})
}

// NewConsumer 要求有限的处理预算，停止服务时不能无限等一条消息。
func NewConsumer(reader MessageReader, handle func(context.Context, OrderCreated) (bool, error), timeout time.Duration, logger *slog.Logger, runtime *telemetry.Runtime) (*Consumer, error) {
	if reader == nil || handle == nil || timeout <= 0 || runtime == nil {
		return nil, fmt.Errorf("消息消费者需要读取器、处理函数和正超时")
	}
	return &Consumer{reader: reader, handle: handle, timeout: timeout, logger: logger, telemetry: runtime}, nil
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
func (consumer *Consumer) process(ctx context.Context, message kafka.Message) (result error) {
	headers := telemetry.KafkaHeaders(message.Headers)
	ctx = consumer.telemetry.Propagator.Extract(ctx, &headers)
	ctx, span := consumer.telemetry.Provider.Tracer("opsdoctor/events").Start(ctx, "order.created process", trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(attribute.String("messaging.system", "kafka"), attribute.String("messaging.destination.name", OrderCreatedTopic)))
	started, outcome := time.Now(), "failed"
	defer func() {
		consumer.telemetry.Metrics.Messages.WithLabelValues(OrderCreatedTopic, outcome).Inc()
		consumer.telemetry.Metrics.ProcessDuration.WithLabelValues(OrderCreatedTopic).Observe(time.Since(started).Seconds())
		if result != nil {
			consumer.logger.ErrorContext(ctx, "订单消息未完成，保留位点等待核对", "partition", message.Partition, "offset", message.Offset)
		}
		telemetry.Finish(span, result)
	}()
	event, err := decodeMessage(message)
	if err != nil {
		return err
	}
	duplicate, err := consumer.handle(ctx, event)
	if err != nil {
		return fmt.Errorf("更新销量：%w", err)
	}
	if err := consumer.reader.CommitMessages(ctx, message); err != nil {
		return fmt.Errorf("提交订单消息位点：%w", err)
	}
	outcome = "processed"
	if duplicate {
		outcome = "duplicate" // 数据库明确返回已处理，不能根据消息的投递次数猜测。
	}
	consumer.logger.InfoContext(ctx, "订单消息已处理并提交位点", "order_id", event.OrderID, "result", outcome)
	return nil
}

// decodeMessage 在开始事务前检查正文和 key，坏消息不能被错记为已消费。
func decodeMessage(message kafka.Message) (OrderCreated, error) {
	var event OrderCreated
	decoder := json.NewDecoder(bytes.NewReader(message.Value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return OrderCreated{}, fmt.Errorf("订单消息 JSON 不合法")
	}
	// 一条 Kafka 消息只能承载一个 JSON 值，尾随第二个对象也应拒绝。
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return OrderCreated{}, fmt.Errorf("订单消息存在多余内容")
	}
	if err := event.Validate(); err != nil {
		return OrderCreated{}, err
	}
	if string(message.Key) != strconv.FormatInt(event.OrderID, 10) {
		return OrderCreated{}, fmt.Errorf("订单消息 key 与订单编号不一致")
	}
	return event, nil
}
