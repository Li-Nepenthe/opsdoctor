// 本文件：验证 SSE 续传、顺序和心跳，不依赖真实端口或真实时间。
//
// 仓储已经有三步，客户端只看过第一步。重连必须补发第二、三步，不能从头重复。
// 另一个测试让连接保持空闲，虚拟时钟走十五秒，确认心跳不会假装成新步骤。
//
// 主要内容：TestReplay 检查 Last-Event-ID；TestHeartbeat 检查保活与取消。
// 阅读建议：ResponseRecorder 是内存 HTTP writer，生产的写入时限由 net/http 连接提供。

package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/events"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
)

// replayStore 在测试开始前固定步骤，处理器只读，不需要额外的并发写入模拟。
type replayStore struct {
	*memoryStore
	steps  []agent.Step
	status string
}

// Detail 提供已保存的最大编号，未来编号不能静默跳过真实内容。
func (memory *replayStore) Detail(_ context.Context, id string) (store.Detail, error) {
	return store.Detail{Diagnosis: store.Diagnosis{ID: id, Status: memory.status}, Steps: memory.steps, LastSeq: len(memory.steps)}, nil
}

// StepsAfter 按数据库契约返回严格递增的后续记录。
func (memory *replayStore) StepsAfter(_ context.Context, _ string, after int) ([]agent.Step, error) {
	var result []agent.Step
	for _, step := range memory.steps {
		if step.Seq > after {
			result = append(result, step)
		}
	}
	return result, nil
}

// TestReplay 以不同续传位置重连，已见步骤不能重复，终态也不能让页面无限自动重连。
func TestReplay(t *testing.T) {
	router, _, token := setupWithOptions(t, func(options *Options) {
		options.Events = events.New()
		options.Store = &replayStore{memoryStore: options.Store.(*memoryStore), status: "succeeded", steps: []agent.Step{{Seq: 1, Kind: "llm", Status: "ok"}, {Seq: 2, Kind: "tool", Status: "ok"}, {Seq: 3, Kind: "system", Status: "succeeded"}}}
	})
	for _, testcase := range []struct {
		after  string
		want   []string
		reject string
		code   int
	}{
		{"1", []string{"id: 2\n", "id: 3\n"}, "id: 1\n", 200},    // 已读第一步，只补后两步。
		{"2", []string{"id: 3\n"}, "id: 2\n", 200},               // 再次断线只缺最后一步。
		{"3", []string{"event: status\n"}, "event: step\n", 200}, // 已读全部，告知结束。
		{"4", nil, "event: step\n", 400},                         // 未来编号不是合法续传。
	} {
		request := httptest.NewRequest("GET", "/api/v1/diagnoses/"+strings.Repeat("a", 32)+"/events", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Last-Event-ID", testcase.after)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != testcase.code {
			t.Fatalf("after=%s code=%d", testcase.after, response.Code)
		}
		for _, want := range testcase.want {
			if strings.Count(response.Body.String(), want) != 1 {
				t.Fatalf("缺少或重复 %q", want)
			}
		}
		if strings.Contains(response.Body.String(), testcase.reject) {
			t.Fatalf("重复补发 after=%s", testcase.after)
		}
	}
}

// TestHeartbeat 在没有新步骤时只发注释心跳；取消请求后处理器必须退出。
func TestHeartbeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		router, _, token := setupWithOptions(t, func(options *Options) {
			options.Events = events.New()
			options.Store = &replayStore{memoryStore: options.Store.(*memoryStore), status: "running"}
		})
		ctx, cancel := context.WithCancel(t.Context())
		request := httptest.NewRequest("GET", "/api/v1/diagnoses/"+strings.Repeat("a", 32)+"/events", nil).WithContext(ctx)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		done := make(chan struct{})
		go func() { router.ServeHTTP(response, request); close(done) }()
		synctest.Wait()
		time.Sleep(15 * time.Second)
		synctest.Wait()
		// Wait 保证 writer 暂停在等待下一次事件，读内存响应时没有数据竞争。
		if !strings.Contains(response.Body.String(), ": heartbeat\n\n") || strings.Contains(response.Body.String(), "id:") {
			t.Fatal("心跳缺失或占了步骤号")
		}
		cancel()
		<-done
	})
}
