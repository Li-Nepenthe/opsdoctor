// 本文件：证明消费者在数据库成功后才提交位点。
//
// 测试用内存消息队列替代 Kafka，能够明确记录处理与提交的先后顺序。
//
// 主要内容：
//   - fakeReader：记录 Fetch 与 Commit，支持模拟提交失败。
//   - TestConsumerCommitOrder：验证失败不提交、重复送达可再次提交。
//   - TestConsumerFinishesCurrentMessage：收到停止信号仍做完当前消息。
//   - TestRejectInvalidMessage：拒绝损坏内容和与订单编号不符的 key。
//
// 阅读建议：先看提交时的断言，它比“函数最后返回 nil”更能证明处理顺序。

package events

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

// fakeReader 在同一个消费 goroutine 中使用，不需要为这些计数器加锁。
type fakeReader struct {
	messages  []kafka.Message       // 顺序提供消息，可以包含同一订单的重复消息。
	fetched   int                   // 下一条消息的位置。
	committed int                   // 成功提交了几次。
	commitErr error                 // 非空时模拟 broker 未确认提交。
	onCommit  func(context.Context) // 提交时检查数据库是否已经完成。
}

// FetchMessage 队列为空时等待取消，模拟消费者平时没有新消息的状态。
func (reader *fakeReader) FetchMessage(ctx context.Context) (kafka.Message, error) {
	if reader.fetched < len(reader.messages) {
		message := reader.messages[reader.fetched]
		reader.fetched++
		return message, nil
	}
	<-ctx.Done()
	return kafka.Message{}, ctx.Err()
}

// CommitMessages 先触发顺序断言，再记录成功，提交失败时不增加已提交数。
func (reader *fakeReader) CommitMessages(ctx context.Context, _ ...kafka.Message) error {
	if reader.onCommit != nil {
		reader.onCommit(ctx)
	}
	if reader.commitErr != nil {
		return reader.commitErr
	}
	reader.committed++
	return nil
}

// testMessage 用真实 JSON 编码生成消息，避免手写字段与契约不一致。
func testMessage(t *testing.T) kafka.Message {
	t.Helper()
	event := OrderCreated{OrderID: 1, UserID: 2, SKUID: 3, Quantity: 4, AmountCents: 100, CreatedAt: time.Now().UTC()}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return kafka.Message{Key: []byte(strconv.FormatInt(event.OrderID, 10)), Value: data, Partition: 1, Offset: 7}
}

// testConsumer 把日志送到 Discard，让预期失败不会混进测试输出。
func testConsumer(t *testing.T, reader *fakeReader, handle func(context.Context, OrderCreated) error) *Consumer {
	t.Helper()
	consumer, err := NewConsumer(reader, handle, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return consumer
}

// TestConsumerCommitOrder 分别模拟数据库失败、位点提交失败和重复消息。
func TestConsumerCommitOrder(t *testing.T) {
	for _, testcase := range []struct {
		name       string // 描述本次在哪一步停止。
		handleErr  error  // 数据库处理是否成功。
		commitErr  error  // Kafka 是否确认提交。
		duplicates bool   // 是否连续送达两份同一订单。
		committed  int    // 真正提交成功的次数。
	}{
		{"数据库失败", errors.New("数据库断开"), nil, false, 0},   // 处理失败不能推进位点。
		{"提交失败", nil, errors.New("Kafka 断开"), false, 0}, // 数据库成功也可能需要重复投递。
		{"重复送达", nil, nil, true, 2},                     // 去重由数据库负责，消费层仍需提交第二条消息的位置。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			reader := &fakeReader{messages: []kafka.Message{testMessage(t)}, commitErr: testcase.commitErr}
			if testcase.duplicates {
				reader.messages = append(reader.messages, testMessage(t))
			}
			handled := 0
			reader.onCommit = func(context.Context) {
				if handled != reader.fetched {
					t.Error("数据库尚未完成就提交位点")
				}
				if reader.fetched == len(reader.messages) {
					cancel()
				}
			}
			consumer := testConsumer(t, reader, func(context.Context, OrderCreated) error { handled++; return testcase.handleErr })
			err := consumer.Run(ctx)
			if (err != nil) != (testcase.handleErr != nil || testcase.commitErr != nil) || reader.committed != testcase.committed {
				t.Fatalf("committed=%d err=%v", reader.committed, err)
			}
		})
	}
}

// TestConsumerFinishesCurrentMessage 在处理数据库时取消服务，当前消息的独立预算仍然有效。
func TestConsumerFinishesCurrentMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reader := &fakeReader{messages: []kafka.Message{testMessage(t), testMessage(t)}}
	reader.onCommit = func(processing context.Context) {
		if processing.Err() != nil {
			t.Error("停止信号提前取消了当前提交")
		}
	}
	consumer := testConsumer(t, reader, func(processing context.Context, _ OrderCreated) error {
		cancel()
		// 当前处理不继承停止信号，但必须有自己的 deadline，不能无限拖住停机。
		if _, ok := processing.Deadline(); !ok || processing.Err() != nil {
			t.Error("当前消息缺少有效预算")
		}
		return nil
	})
	if err := consumer.Run(ctx); err != nil || reader.committed != 1 || reader.fetched != 1 {
		t.Fatalf("未完成手上消息或又取了新消息：%+v %v", reader, err)
	}
}

// TestRejectInvalidMessage 坏消息不提交，避免静默丢弃后看起来消费一切正常。
func TestRejectInvalidMessage(t *testing.T) {
	for _, testcase := range []struct {
		name   string               // 描述消息被拒绝的理由。
		change func(*kafka.Message) // 在合法消息上只改动一个条件。
	}{
		{"损坏 JSON", func(message *kafka.Message) { message.Value = []byte("{") }},                          // 无法解析时不进业务。
		{"尾随对象", func(message *kafka.Message) { message.Value = append(message.Value, []byte(" {}")...) }}, // 每条消息只允许一个对象。
		{"错误 key", func(message *kafka.Message) { message.Key = []byte("2") }},                             // key 应与订单 ID 一致。
		{"缺少字段", func(message *kafka.Message) { message.Value = []byte(`{"order_id":1}`) }},                // 缺少 SKU 和数量不能记账。
	} {
		t.Run(testcase.name, func(t *testing.T) {
			message := testMessage(t)
			testcase.change(&message)
			reader := &fakeReader{}
			consumer := testConsumer(t, reader, func(context.Context, OrderCreated) error { t.Error("坏消息进入业务处理"); return nil })
			if err := consumer.process(t.Context(), message); err == nil || reader.committed != 0 {
				t.Fatalf("坏消息被提交：%v", err)
			}
		})
	}
}
