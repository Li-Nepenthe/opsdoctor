// 本文件：复现“调用者结束等待，Kafka 批次随后才确认”的时间顺序。
//
// 旧告警里的订单后来都被消费，这并不能证明当时收到了发送确认。
// 测试保留真实 kafka.Writer，只把网络传输换成内存对象，手动控制确认何时到达。
//
// 主要内容：
//   - waitingBroker.RoundTrip：返回分区信息，并等测试允许后再确认发送。
//   - TestPublisherCancellationDoesNotCancelBatch：证明两个 context 的生命周期不同。
//
// 阅读建议：按“进入发送 → 取消调用者 → 调用返回 → 允许批次确认”读主测试。

package events

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/protocol/metadata"
	"github.com/segmentio/kafka-go/protocol/produce"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// waitingBroker 的通道把时序交给测试控制，不用真实网络延迟碰运气。
type waitingBroker struct {
	entered chan struct{}   // 发送协程通知主测试已开始等待确认，容量 1 避免通知反过来卡住发送。
	allow   <-chan struct{} // 主测试关闭后，内存 broker 才返回成功。
}

// RoundTrip 实现 kafka.RoundTripper；这相当于把 Java 客户端的传输实现换成测试替身。
func (broker *waitingBroker) RoundTrip(ctx context.Context, _ net.Addr, request kafka.Request) (kafka.Response, error) {
	switch request.(type) {
	case *metadata.Request:
		return &metadata.Response{Topics: []metadata.ResponseTopic{{Name: OrderCreatedTopic, Partitions: []metadata.ResponsePartition{{PartitionIndex: 0}}}}}, nil
	case *produce.Request:
		broker.entered <- struct{}{}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-broker.allow:
			return &produce.Response{Topics: []produce.ResponseTopic{{Topic: OrderCreatedTopic, Partitions: []produce.ResponsePartition{{Partition: 0}}}}}, nil
		}
	default:
		return nil, fmt.Errorf("测试没有实现请求 %T", request)
	}
}

// TestPublisherCancellationDoesNotCancelBatch 证明未收到确认时仍需告警，不能因为消息以后可能送达就直接记成功。
func TestPublisherCancellationDoesNotCancelBatch(t *testing.T) {
	const testBudget = 3 * time.Second // 防止实现变化导致测试无限等待，不作为性能断言。
	testCtx, stop := context.WithTimeout(t.Context(), testBudget)
	defer stop()
	runtime := &telemetry.Runtime{Metrics: telemetry.NewMetrics("test"), Provider: sdktrace.NewTracerProvider(), Propagator: propagation.TraceContext{}}
	t.Cleanup(func() { _ = runtime.Provider.Shutdown(context.Background()) })
	publisher := NewPublisher([]string{"memory:9092"}, runtime, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { _ = publisher.Close() })
	gate := make(chan struct{}) // 只有主测试关闭；底层发送协程只读。
	var releaseOnce sync.Once   // 正常路径与失败清理可能都要放行，只关闭一次。
	release := func() { releaseOnce.Do(func() { close(gate) }) }
	t.Cleanup(release)
	broker := &waitingBroker{entered: make(chan struct{}, 1), allow: gate}
	publisher.writer.Transport = broker
	confirmed := make(chan error, 1) // Writer 的完成回调写一次，主测试核对最终结果。
	publisher.writer.Completion = func(_ []kafka.Message, err error) { confirmed <- err }
	caller, cancel := context.WithCancel(testCtx)
	defer cancel()
	returned := make(chan error, 1) // Publish 结束后写入，容量 1 保证测试失败时也能退出。
	go func() {
		returned <- publisher.Publish(caller, OrderCreated{OrderID: 1, UserID: 1, SKUID: 1, Quantity: 1, CreatedAt: time.Now().UTC()})
	}()
	// 先证明批次真的进入传输，再取消调用者；否则可能只测试了“发送前取消”。
	select {
	case <-broker.entered:
	case <-testCtx.Done():
		t.Fatal("没有进入发送")
	}
	cancel()
	if err := <-returned; !errors.Is(err, context.Canceled) {
		t.Fatalf("调用者没有按取消返回：%v", err)
	}
	// 此刻 Publish 已返回，但 Writer 的独立 context 仍有效，确认还可以到达。
	release()
	select {
	case err := <-confirmed:
		if err != nil {
			t.Fatalf("后台批次没有完成：%v", err)
		}
	case <-testCtx.Done():
		t.Fatal("没有收到后台完成回调")
	}
}
