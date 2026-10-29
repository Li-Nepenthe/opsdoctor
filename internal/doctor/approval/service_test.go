// 本文件：验证审批保存以后，模型不能换掉执行参数。
//
// 测试服务器充当固定运维入口，直接记录收到的商品编号和修改次数。内存仓储只
// 模拟已经持久化的审批；真实数据库的竞争条件由 store 的集成测试另行验证。
//
// 主要内容：TestSavedArguments 检查原单执行与新单申请；TestPoolRecovery 检查回复丢失后的核对。
// 阅读建议：先看原始参数与篡改参数，再看服务器究竟收到了哪一份。

package approval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// memoryApprovals 保存独立单子，不允许第二次 Handle 重新覆盖原始参数。
type memoryApprovals map[string]Record

// FindApproval 只读取现有单子，缺失才允许准备新申请。
func (repo memoryApprovals) FindApproval(_ context.Context, _ string, id string) (Record, error) {
	value, ok := repo[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	return value, nil
}

// CreateApproval 记录准备阶段的确切参数，此后只能改变状态和执行结果。
func (repo memoryApprovals) CreateApproval(_ context.Context, value Record) (Record, error) {
	value.Status = "pending"
	repo[value.ID] = value
	return value, nil
}

// ClaimApproval 模拟持久层交给执行器的已领取原单。
func (repo memoryApprovals) ClaimApproval(_ context.Context, _ string, id string) (Record, error) {
	value := repo[id]
	value.Status = "executing"
	repo[id] = value
	return value, nil
}

// ApprovalConnection 在这些纯 HTTP 用例中不应使用，保留接口以避免另一条执行路径。
func (memoryApprovals) ApprovalConnection(context.Context, string, string, int64) error { return nil }

// FinishApproval 保存结果供后续重放，不能因调用方重试再修改远端。
func (repo memoryApprovals) FinishApproval(_ context.Context, _ string, id string, result tools.Result) error {
	value := repo[id]
	value.Status = "executed"
	value.Result = result
	repo[id] = value
	return nil
}

// TestSavedArguments 用已批准的缓存预热证明模型后来的参数不会进入执行器。
func TestSavedArguments(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		var input struct {
			IDs []int `json:"ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&input)
		if len(input.IDs) != 1 || input.IDs[0] != 1 {
			t.Error("执行了篡改后的参数")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"loaded":1,"already_present":0}`))
	}))
	defer server.Close()
	backend, err := NewBackend("", nil, nil, OpsConfig{Services: map[string]string{"order": server.URL}, Token: strings.Repeat("a", 32), WarmService: "order"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	repo := memoryApprovals{}
	service := New("shop", backend, repo, llm.NewRedactor())
	args := `{"ids":[1],"reason":"已有观察","evidence_steps":[1],"expected_effect":"减少回源","risk":"额外查询"}`
	call := llm.ToolCall{ID: "first", Type: "function", Function: llm.Function{Name: "warm_cache", Arguments: args}}
	waiting, _, err := service.Handle(t.Context(), "diagnosis", 2, call, map[int]bool{1: true})
	if err != nil || !waiting || posts != 0 {
		t.Fatal("准备阶段提前执行")
	}
	// 模拟人批准后，调用者把商品 1 换成 999。执行器必须仍读取已保存的 1。
	for id, value := range repo {
		value.Status = "approved"
		repo[id] = value
	}
	call.Function.Arguments = strings.Replace(args, `"ids":[1]`, `"ids":[999]`, 1)
	for range 2 {
		waiting, result, err := service.Handle(t.Context(), "diagnosis", 2, call, map[int]bool{1: true})
		if err != nil || waiting || result.Status != "ok" {
			t.Fatalf("执行失败：%v %+v", err, result)
		}
	}
	if posts != 1 {
		t.Fatal("原单重放再次执行")
	}
	call.ID = "second"
	waiting, _, err = service.Handle(t.Context(), "diagnosis", 2, call, map[int]bool{1: true})
	if err != nil || !waiting || len(repo) != 2 || posts != 1 {
		t.Fatal("换调用没有创建待审批的新单")
	}
}

// TestPoolRecovery 已执行、外部修改和进程重启都只能读取确认，不能盲目再次 POST。
func TestPoolRecovery(t *testing.T) {
	for _, test := range []struct {
		name    string
		state   PoolState
		success bool
	}{
		{"旧回复丢失", PoolState{Instance: "old", Current: 30, Revision: 1, LastOperation: "approval"}, true},
		{"别的操作改过", PoolState{Instance: "old", Current: 40, Revision: 1, LastOperation: "other"}, false},
		{"进程已重启", PoolState{Instance: "new", Current: 20}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			// 三种情况都不能发写请求：第一种已完成，后两种审批依据已失效。
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Error("恢复盲目写入")
				}
				_ = json.NewEncoder(w).Encode(test.state)
			}))
			defer server.Close()
			backend, _ := NewBackend("", nil, nil, OpsConfig{Services: map[string]string{"inventory": server.URL}, Token: strings.Repeat("a", 32)}, time.Minute)
			defer backend.Close()
			record := Record{ID: "approval", Tool: "set_db_pool_size", Status: "executing", Plan: Plan{Service: "inventory", Size: 30, Before: PoolState{Instance: "old", Current: 20}}}
			_, err := backend.setPool(t.Context(), record)
			if (err == nil) != test.success {
				t.Fatalf("结果错误：%v", err)
			}
		})
	}
}
