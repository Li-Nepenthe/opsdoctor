// 本文件：用内存订阅核对诊断隔离和慢订阅者断开。断开前缓冲的 64 条通知仍可读，断线补发由 HTTP 测试覆盖。

package events

import (
	"encoding/json"
	"testing"
)

// TestSlowSubscriber 持续读取的一方不受另一方阻塞影响，别的诊断完全收不到事件。
func TestSlowSubscriber(t *testing.T) {
	hub := New()
	slow, fast, other := hub.Subscribe("a"), hub.Subscribe("a"), hub.Subscribe("b")
	defer other.Close()
	defer fast.Close()
	for seq := 1; seq <= BufferSize+1; seq++ {
		hub.Publish("a", Event{ID: seq, Data: json.RawMessage(`{}`)})
		if event := <-fast.Events(); event.ID != seq {
			t.Fatal("快订阅者漏消息")
		}
	}
	count := 0
	for range slow.Events() {
		count++
	}
	if count != BufferSize {
		t.Fatalf("缓冲数=%d", count)
	}
	slow.Close() // 满通道已经关闭过，再关闭不能 panic。
	select {
	case <-other.Events():
		t.Fatal("诊断之间串消息")
	default:
	}
}
