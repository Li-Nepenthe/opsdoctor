// 本文件：验证 Kafka 消费组冷启动时的暂态错误不会直接终止消费。
//
// 测试只替换 MessageReader，不启动 Kafka，也不改变销量事务。这里关注的边界是：
// 尚未取到消息时可以等协调器恢复，已经拿到消息后的失败仍由原消费循环处理。
//
// 阅读建议：先看错误 16 的用例，再看预算与取消，理解“允许恢复”仍然有明确边界。

package events

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

// fetchErrorReader 的脚本中，nil 表示交给原内存队列取一条消息，非 nil 则只返回错误。
type fetchErrorReader struct {
	fakeReader                       // 复用现有测试队列的消息和提交记录，不引入第二套提交实现。
	errors     []error               // 按 FetchMessage 调用顺序消费，nil 表示本次可以取消息。
	calls      int                   // 包含失败的读取次数，防止重试越过预算。
	onError    func()                // 取消测试用它确认第一次错误已经返回。
	onRead     func(context.Context) // 正常读取前检查传入的 context；不改变消息与提交行为。
}

// FetchMessage 先返回脚本中的错误；错误不推进消息队列的位置。
func (reader *fetchErrorReader) FetchMessage(ctx context.Context) (kafka.Message, error) {
	reader.calls++
	if len(reader.errors) > 0 {
		err := reader.errors[0]
		reader.errors = reader.errors[1:]
		if err != nil {
			if reader.onError != nil {
				reader.onError()
			}
			return kafka.Message{}, err
		}
	}
	if reader.onRead != nil {
		reader.onRead(ctx)
	}
	return reader.fakeReader.FetchMessage(ctx)
}

// TestConsumerRetriesCoordinatorErrors 覆盖同类初始化状态，尤其是实测出现的错误 16。
func TestConsumerRetriesCoordinatorErrors(t *testing.T) {
	for _, protocolError := range []error{kafka.GroupLoadInProgress, kafka.GroupCoordinatorNotAvailable, fmt.Errorf("入组：%w", kafka.NotCoordinatorForGroup)} {
		t.Run(protocolError.Error(), func(t *testing.T) {
			// 错误没有交出消息；恢复之后才允许这唯一的一条订单进入处理和提交。
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			reader := &fetchErrorReader{fakeReader: fakeReader{messages: []kafka.Message{testMessage(t)}}, errors: []error{protocolError}}
			reader.onCommit = func(context.Context) { cancel() }
			handled := 0
			consumer := testConsumer(t, reader, func(context.Context, OrderCreated) error { handled++; return nil })
			if err := consumer.Run(ctx); err != nil {
				t.Fatal(err)
			}
			if reader.calls != 2 || reader.fetched != 1 || reader.committed != 1 || handled != 1 {
				t.Fatalf("恢复后应只处理和提交一次：calls=%d fetched=%d committed=%d handled=%d", reader.calls, reader.fetched, reader.committed, handled)
			}
		})
	}
}

// TestConsumerPermanentFetchError 防止扩大成所有错误都重试，掩盖账号或协议配置问题。
func TestConsumerPermanentFetchError(t *testing.T) {
	for _, permanent := range []error{kafka.GroupAuthorizationFailed, kafka.UnsupportedVersion, errors.New("读取配置失败")} {
		reader := &fetchErrorReader{errors: []error{permanent}}
		consumer := testConsumer(t, reader, func(context.Context, OrderCreated) error { t.Error("错误不应进入业务处理"); return nil })
		if err := consumer.Run(t.Context()); !errors.Is(err, permanent) || reader.calls != 1 || reader.committed != 0 {
			t.Fatalf("不可恢复错误被吞掉或重试：calls=%d err=%v", reader.calls, err)
		}
	}
}

// TestConsumerCoordinatorRetryBudget 验证持续失败只能重取三次，不能无限等一个坏配置。
func TestConsumerCoordinatorRetryBudget(t *testing.T) {
	reader := &fetchErrorReader{}
	for range coordinatorMaxRetries + 1 {
		reader.errors = append(reader.errors, kafka.NotCoordinatorForGroup)
	}
	consumer := testConsumer(t, reader, func(context.Context, OrderCreated) error {
		t.Error("协调器失败不应进入业务处理")
		return nil
	})
	err := consumer.Run(t.Context())
	if !errors.Is(err, kafka.NotCoordinatorForGroup) || !strings.Contains(err.Error(), "重试预算耗尽") || reader.calls != coordinatorMaxRetries+1 || reader.committed != 0 {
		t.Fatalf("持续错误没有按预算结束：calls=%d err=%v", reader.calls, err)
	}
}

// TestConsumerCoordinatorBudgetResets 保证两次独立恢复不会错误地共用一个进程终身次数。
func TestConsumerCoordinatorBudgetResets(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reader := &fetchErrorReader{fakeReader: fakeReader{messages: []kafka.Message{testMessage(t), testMessage(t)}}}
	// 每个错误段都用满三次重取；若错误地全程累计，第二条消息到来之前就会退出。
	for range 2 {
		for range coordinatorMaxRetries {
			reader.errors = append(reader.errors, kafka.NotCoordinatorForGroup)
		}
		reader.errors = append(reader.errors, nil)
	}
	reader.onCommit = func(context.Context) {
		if reader.fetched == 2 {
			cancel()
		}
	}
	consumer := testConsumer(t, reader, func(context.Context, OrderCreated) error { return nil })
	if err := consumer.Run(ctx); err != nil || reader.committed != 2 || reader.calls != 2*(coordinatorMaxRetries+1) {
		t.Fatalf("成功读取后没有重置预算：calls=%d committed=%d err=%v", reader.calls, reader.committed, err)
	}
}

// TestConsumerCoordinatorRecoveryIdleContext 防止恢复后安静的主题因人为新增短 deadline 被错杀。
func TestConsumerCoordinatorRecoveryIdleContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{}) // 正常读取会等待消息；测试收到信号后取消这次空闲等待。
	reader := &fetchErrorReader{errors: []error{kafka.NotCoordinatorForGroup}}
	reader.onRead = func(reading context.Context) {
		if _, exists := reading.Deadline(); exists {
			t.Error("协调器恢复后的正常读取被附加了 deadline")
		}
		close(entered)
	}
	consumer := testConsumer(t, reader, func(context.Context, OrderCreated) error { t.Error("空闲读取没有消息可处理"); return nil })
	finished := make(chan error, 1) // Run 退出后发送结果，测试通过接收建立同步，再读取计数。
	go func() { finished <- consumer.Run(ctx) }()
	select {
	case <-entered:
		cancel()
	case <-time.After(time.Second): // 一次 250ms 退避之后应该已经进入正常读取。
		t.Fatal("没有进入恢复后的空闲读取")
	}
	select {
	case err := <-finished:
		if err != nil || reader.calls != 2 || reader.fetched != 0 || reader.committed != 0 {
			t.Fatalf("空闲读取取消没有退出：calls=%d err=%v", reader.calls, err)
		}
	case <-time.After(time.Second): // 原 context 的取消应打断空闲 Fetch，而不是等一份额外预算。
		t.Fatal("空闲读取没有响应取消")
	}
}

// TestConsumerCoordinatorErrorAfterFetch 保证错误码相同也不能把业务处理或提交失败当成尚未取到消息。
func TestConsumerCoordinatorErrorAfterFetch(t *testing.T) {
	for _, commitFails := range []bool{false, true} {
		reader := &fetchErrorReader{fakeReader: fakeReader{messages: []kafka.Message{testMessage(t), testMessage(t)}}}
		if commitFails {
			reader.commitErr = kafka.NotCoordinatorForGroup
		}
		handled := 0
		consumer := testConsumer(t, reader, func(context.Context, OrderCreated) error {
			handled++
			if !commitFails {
				return kafka.NotCoordinatorForGroup
			}
			return nil
		})
		// 队列里故意留第二条，若重试包错了层，这条断言能发现跳过未确认消息的行为。
		err := consumer.Run(t.Context())
		if !errors.Is(err, kafka.NotCoordinatorForGroup) || reader.calls != 1 || reader.fetched != 1 || reader.committed != 0 || handled != 1 {
			t.Fatalf("已取得消息后的失败被错误重取：commitFails=%v calls=%d fetched=%d committed=%d handled=%d err=%v", commitFails, reader.calls, reader.fetched, reader.committed, handled, err)
		}
	}
}

// TestConsumerCancelCoordinatorRetry 验证停止信号不会触发第二次读取，更不会处理消息。
func TestConsumerCancelCoordinatorRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	failed := make(chan struct{}) // 消费 goroutine 只写一次；测试收到后发出停止信号。
	reader := &fetchErrorReader{errors: []error{kafka.NotCoordinatorForGroup}, onError: func() { close(failed) }}
	consumer := testConsumer(t, reader, func(context.Context, OrderCreated) error { t.Error("取消后不能处理消息"); return nil })
	finished := make(chan error, 1) // 即使测试超时，消费 goroutine 返回时也不被发送结果阻塞。
	go func() { finished <- consumer.Run(ctx) }()
	<-failed
	cancel()
	select {
	case err := <-finished:
		if err != nil || reader.calls != 1 || reader.committed != 0 {
			t.Fatalf("取消后仍继续读取：calls=%d err=%v", reader.calls, err)
		}
	case <-time.After(time.Second): // 正常取消不需要等一次退避，更不应达到一秒。
		t.Fatal("恢复退避没有响应取消")
	}
}

// TestFetchRetryWaitCancellation 用很长的待等待值证明取消会打断计时器，测试本身不会等待该时长。
func TestFetchRetryWaitCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan error, 1) // 测试负责取消，后台等待函数负责返回并退出。
	go func() { finished <- waitFetchRetry(ctx, time.Hour) }()
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("取消原因丢失：%v", err)
		}
	case <-time.After(time.Second): // 和一小时的假定退避相比，一秒足以区分能否被打断。
		t.Fatal("计时器没有及时响应取消")
	}
}
