// 本文件：检查实时订阅不会串诊断，也不会被慢浏览器拖住。
//
// 这里只操作内存通道，数据库补发由 httpapi 的 SSE 测试覆盖。
// 主要内容：TestSlowSubscriber 验证固定缓冲、断开和重复关闭。
// 阅读建议：满通道断开后，已有的 64 条仍可被读完，再读才得到关闭状态。

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
