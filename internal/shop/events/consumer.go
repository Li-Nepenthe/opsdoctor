// 本文件：消费者先提交销量事务，再提交 Kafka 位点。坏消息留在原位，避免重启后跳过未入账的订单。
// 从 Run 进入 process，可以看到事务与位点各自的失败由谁处理。

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

// MessageReader 把取消息和提交位点分开，消费者控制提交时机。
type MessageReader interface {
	FetchMessage(context.Context) (kafka.Message, error)
	CommitMessages(context.Context, ...kafka.Message) error
}

// 拉取限额控制等待与内存，不改变提交位点的时机。
const (
	consumeMaxBytes            = 1 << 20                // 正常事件只有几个字段，一 MiB 足以容纳一批消息。
	consumeMaxWait             = 500 * time.Millisecond // 低流量下最多等半秒，不为凑批长时间等待。
	coordinatorMaxRetries      = 3                      // 读取器连续三次入组失败才上报；上层收到协调器错误后最多重取三次。
	coordinatorRetryBackoff    = 250 * time.Millisecond // 避免读取器快速报错时形成忙循环，不与库的入组定时器抢跑。
	coordinatorRetryMaxBackoff = time.Second            // 退避依次为 250ms、500ms、1s，停机不必等完这段时间。
)

// Consumer 不拥有 reader 的关闭权，app 等 Run 返回后统一关闭连接。
type Consumer struct {
	reader    MessageReader
	handle    func(context.Context, OrderCreated) (bool, error) // bool 表示重复；无错误才允许提交位点。
	timeout   time.Duration                                     // 处理一条消息与提交位点共享这份预算。
	logger    *slog.Logger
	telemetry *telemetry.Runtime
}

// NewReader 从消费组已提交的位置继续读取，新组则从最早保留的消息开始。
func NewReader(brokers []string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, Topic: OrderCreatedTopic, GroupID: "order-worker",
		StartOffset: kafka.FirstOffset, CommitInterval: 0, MinBytes: 1, MaxBytes: consumeMaxBytes,
		MaxWait: consumeMaxWait, WatchPartitionChanges: true})
}

// NewConsumer 要求有限的处理预算，停止服务时不能无限等一条消息。
func NewConsumer(
	reader MessageReader,
	handle func(context.Context, OrderCreated) (bool, error),
	timeout time.Duration,
	logger *slog.Logger,
	runtime *telemetry.Runtime,
) (*Consumer, error) {
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
		message, err := consumer.fetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("读取订单消息：%w", err)
		}
		// 停止取新消息时，给手上的消息另留有限时间处理并提交位点。
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

// fetchMessage 只在尚未取得消息时重取；已经取到的消息失败时由消费循环退出。
// 读取器报告入组失败后，仍在后台寻找协调器。协议错误 14、15、16 允许沿用同一读取器有限重试。
// 每条消息单独计算连续错误次数。FetchMessage 本身不设短时限，恢复后没有新订单时应继续等待。
func (consumer *Consumer) fetchMessage(ctx context.Context) (kafka.Message, error) {
	delay := coordinatorRetryBackoff
	for retries := 0; ; retries++ {
		message, err := consumer.reader.FetchMessage(ctx)
		if err == nil || ctx.Err() != nil || !recoverableCoordinatorError(err) {
			return message, err
		}
		if retries >= coordinatorMaxRetries {
			return kafka.Message{}, fmt.Errorf("等待 Kafka 消费组协调器恢复的重试预算耗尽：%w", err)
		}
		// 错误已限定为 Kafka 固定协议错误，不输出连接串、消息正文或账号信息。
		consumer.logger.Warn("Kafka 消费组协调器暂未就绪，等待原读取器恢复", "retry", retries+1, "backoff", delay.String(), "error", err)
		if waitErr := waitFetchRetry(ctx, delay); waitErr != nil {
			return kafka.Message{}, errors.Join(waitErr, err)
		}
		delay *= 2
		if delay > coordinatorRetryMaxBackoff {
			delay = coordinatorRetryMaxBackoff
		}
	}
}

// recoverableCoordinatorError 只接受协调器暂未就绪的协议错误。
func recoverableCoordinatorError(err error) bool {
	return errors.Is(err, kafka.GroupLoadInProgress) || errors.Is(err, kafka.GroupCoordinatorNotAvailable) || errors.Is(err, kafka.NotCoordinatorForGroup)
}

// waitFetchRetry 让停机信号能打断重取退避。
func waitFetchRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// process 处理成功后才提交位点；提交失败的订单再次送达时由销量事务去重。
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
