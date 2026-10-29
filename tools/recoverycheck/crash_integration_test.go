// 本文件：真的终止一个子进程，再用同一个测试库恢复诊断。
//
// 普通事务测试可以模拟租约过期，但不能证明进程消失时内存数据不会被误当成
// 已保存的步骤。这里运行当前测试程序的子进程，只使用 Mock 和专用医生测试库。
//
// 主要内容：TestIntegrationProcessCrash 覆盖查证、换模型、待审批和中断过久；
// TestIntegrationCrashWorker 是子进程入口；crashModel 提供确定的离线回答。
// TestIntegrationWriteCrash 在运维接口改好连接池、回复尚未送达时杀进程，再核对恢复只读状态。
// 阅读建议：父进程只终止自己启动的子进程，不操作 Docker 或其他应用。

package recoverycheck

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/approval"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/go-sql-driver/mysql"
)

// crashRecorder 使用真实仓储，子进程被杀后不会再补写内存里的步骤。
type crashRecorder struct {
	repository *crashRepository
	id         string
}

// Record 保留生产的幂等键和租约检查，不以写文件替代数据库断点。
func (r crashRecorder) Record(ctx context.Context, step agent.Step) (agent.Step, error) {
	return r.repository.RecordStep(ctx, r.id, step)
}

// crashModel 在第二次调用处停住，让父进程选择一个已明确到达的中断位置。
type crashModel struct {
	mode, model string
	resumed     bool
}

// Complete 从真实工具回复读取证据号；恢复后只交报告，不联网也不重新读取已保存证据。
func (model crashModel) Complete(ctx context.Context, request llm.Request) (llm.Response, error) {
	seq := 0
	for _, message := range request.Messages {
		if message.Role == "tool" {
			var payload struct {
				Step int `json:"step"`
			}
			_ = json.Unmarshal([]byte(message.Content), &payload)
			seq = payload.Step
		}
	}
	name, args, id := "service_topology", "{}", "read-1"
	if model.resumed {
		name, id = "submit_report", "report-1"
		raw, _ := json.Marshal(map[string]any{"summary": "Mock 恢复验收", "root_cause": map[string]any{"component": "unknown", "mechanism": "仅验证断点", "confidence": 0}, "evidence": []any{map[string]any{"step": seq, "observation": "保存的工具结果"}}, "suggested_actions": []any{}, "unknowns": "不是故障判断"})
		args = string(raw)
	} else if seq > 0 {
		if model.mode != "waiting" {
			// 发送意图已经由 Agent 落库，随后进程被杀；没有机会保存这个模型的回复。
			fmt.Println("CRASH_READY")
			<-ctx.Done()
			return llm.Response{}, ctx.Err()
		}
		name, id = "warm_cache", "write-1"
		raw, _ := json.Marshal(map[string]any{"ids": []int{1}, "reason": "Mock 审批验收", "evidence_steps": []int{seq}, "expected_effect": "仅验证等待", "risk": "测试不会批准执行"})
		args = string(raw)
	}
	return llm.Response{Model: model.model, Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: id, Type: "function", Function: llm.Function{Name: name, Arguments: args}}}}}, nil
}

// crashInput 为两边提供同一个架构与只读注册表，避免把恢复误做成另一项新诊断。
func crashInput(t *testing.T, repository *crashRepository, target, id string) agent.Input {
	t.Helper()
	backend, err := approval.NewBackend("", nil, nil, approval.OpsConfig{Services: map[string]string{"fictional": "http://example.invalid"}, Token: strings.Repeat("a", 32), WarmService: "fictional"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	return agent.Input{ID: id, Profile: "虚构系统，只验证断点", Components: []string{"unknown"}, Symptoms: "离线验收", Tools: tools.New(tools.Dependencies{Profile: "虚构系统"}), Recorder: crashRecorder{repository, id}, Approvals: approval.New(target, backend, repository, llm.NewRedactor())}
}

// TestIntegrationCrashWorker 只有父测试给出专用标记才工作，普通测试发现它时直接返回。
func TestIntegrationCrashWorker(t *testing.T) {
	mode := os.Getenv("OPSDOCTOR_TEST_CRASH_MODE")
	if mode == "" {
		return
	}
	dsn := os.Getenv("TEST_DOCTOR_MYSQL_DSN")
	config, err := mysql.ParseDSN(dsn)
	if err != nil || config.DBName != "opsdoctor_doctor_test" {
		t.Fatal("子进程只允许专用测试库")
	}
	database, err := mysqlx.Open(t.Context(), mysqlx.Config{DSN: dsn, MaxOpenConns: 3, MaxIdleConns: 1, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal("测试库连接失败")
	}
	defer database.Close()
	repository := &crashRepository{store.NewConfigured(database, store.Config{LeaseTTL: 2 * time.Second}), database}
	id, target := os.Getenv("OPSDOCTOR_TEST_CRASH_ID"), os.Getenv("OPSDOCTOR_TEST_CRASH_TARGET")
	ctx := llm.WithLease(t.Context(), "child-old")
	if ok, err := repository.StartTask(ctx, id, "mock-old", "crash-test", "mock"); err != nil || !ok {
		t.Fatal("子进程未取得租约")
	}
	if mode == "write" {
		backend := crashPoolBackend(t, os.Getenv("OPSDOCTOR_TEST_OPS_URL"))
		service := approval.New(target, backend, repository, llm.NewRedactor())
		// 父进程的 HTTP 服务会在改好状态后卡住回复，因此这里应被终止，不能正常返回。
		_, _, _ = service.Handle(ctx, id, 1, crashPoolCall(), map[int]bool{1: true})
		t.Fatal("执行中的子进程未被终止")
	}
	loop, _ := agent.New(crashModel{mode: mode, model: "mock-old"}, "mock-old", llm.NewRedactor(), agent.DefaultLimits())
	result, err := loop.Run(ctx, crashInput(t, repository, target, id))
	if err != nil {
		t.Fatal(err)
	}
	if !result.WaitingApproval || mode != "waiting" {
		t.Fatal("没有走到预期的等待点")
	}
	if err = repository.EndTask(ctx, id, run.WaitingApproval, result, "等待测试决定"); err != nil {
		t.Fatal(err)
	}
	// 等待状态已经落库，再通知父进程终止；不能靠任意 sleep 猜测保存是否完成。
	fmt.Println("CRASH_READY")
	<-ctx.Done()
}

// killCrashWorker 只在子进程报告断点已保存之后终止它，并收回句柄。
func killCrashWorker(t *testing.T, mode, id, target string, external <-chan bool, extraEnv ...string) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary, "-test.run=^TestIntegrationCrashWorker$", "-test.timeout=20s")
	command.Env = append(os.Environ(), "OPSDOCTOR_TEST_CRASH_MODE="+mode, "OPSDOCTOR_TEST_CRASH_ID="+id, "OPSDOCTOR_TEST_CRASH_TARGET="+target)
	command.Env = append(command.Env, extraEnv...)
	pipe, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = command.Stdout
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	ready := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) == "CRASH_READY" {
				ready <- true
				return
			}
		}
		ready <- false
	}()
	// 超时只负责使失败结束，成功条件始终是明确的跨进程信号。
	select {
	case <-external:
		// 写操作用远端已修改状态的信号作为断点；子进程此时还收不到 HTTP 回复。
	case ok := <-ready:
		if !ok {
			t.Fatal("子进程未到达断点")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("子进程准备断点超时")
	}
	if err = command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err = command.Wait(); err == nil {
		t.Fatal("子进程未被异常终止")
	}
}

// TestIntegrationProcessCrash 每个用例都实际终止进程，恢复时只使用 MySQL 中留下的记录。
func TestIntegrationProcessCrash(t *testing.T) {
	for _, mode := range []string{"model", "changed-model", "waiting", "too-old"} {
		t.Run(mode, func(t *testing.T) {
			repository, target := integrationStore(t)
			// 测试仓储固定两秒租约，父进程等数据库自然判定过期。
			id, err := repository.Manual(t.Context(), target, "进程崩溃验收")
			if err != nil {
				t.Fatal(err)
			}
			started := time.Now()
			killCrashWorker(t, mode, id, target, nil)
			// 等待人决定没有活租约；其他情况等数据库判定原租约真实过期，不修改时间字段。
			if mode == "waiting" {
				if err = repository.Recover(t.Context()); err != nil {
					t.Fatal(err)
				}
				detail, err := repository.Detail(t.Context(), id)
				if err != nil || detail.Status != run.WaitingApproval {
					t.Fatal("重启丢失待审批状态")
				}
				items, err := repository.Approvals(t.Context(), id)
				if err != nil || len(items) != 1 {
					t.Fatal("审批重复或丢失")
				}
				if ok, err := repository.DecideApproval(t.Context(), items[0].ID, "test-user", false); err != nil || !ok {
					t.Fatal("无法拒绝原单")
				}
			} else {
				deadline := time.Now().Add(5 * time.Second)
				for {
					var expired bool
					if err = repository.database.QueryRowContext(t.Context(), "SELECT lease_until<=UTC_TIMESTAMP(6) FROM diagnoses WHERE id=?", id).Scan(&expired); err != nil {
						t.Fatal(err)
					}
					if expired {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("租约未过期")
					}
					time.Sleep(20 * time.Millisecond)
				}
			}
			if mode == "too-old" {
				repository.Store = store.NewConfigured(repository.database, store.Config{LeaseTTL: 2 * time.Second, ResumeMaxAge: 100 * time.Millisecond})
				if err = repository.Recover(t.Context()); err != nil {
					t.Fatal(err)
				}
				detail, err := repository.Detail(t.Context(), id)
				if err != nil || detail.Status != run.Failed || !strings.Contains(detail.Error, "中断时间超过") {
					t.Fatal("过时断点仍被继续")
				}
				t.Logf("真实终止后拒绝过时恢复，耗时 %s", time.Since(started))
				return
			}
			ctx := llm.WithLease(t.Context(), "parent-new")
			model := "mock-old"
			if mode == "changed-model" {
				model = "mock-new"
			}
			if ok, err := repository.StartTask(ctx, id, model, "crash-test", "mock"); err != nil || !ok {
				t.Fatal("新进程无法接手")
			}
			history, active, cost, err := repository.Resume(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			input := crashInput(t, repository, target, id)
			input.History, input.ActiveTime, input.KnownCost = history, active, cost
			loop, _ := agent.New(crashModel{model: model, resumed: true}, model, llm.NewRedactor(), agent.DefaultLimits())
			result, err := loop.Run(ctx, input)
			if err != nil || len(result.Report) == 0 {
				t.Fatalf("恢复失败：%v %+v", err, result)
			}
			if err = repository.EndTask(ctx, id, run.Succeeded, result, ""); err != nil {
				t.Fatal(err)
			}
			detail, err := repository.Detail(t.Context(), id)
			if err != nil || detail.Model != "mock-old" {
				t.Fatal("初始模型被覆盖")
			}
			steps, err := repository.StepsAfter(t.Context(), id, 0)
			if err != nil {
				t.Fatal(err)
			}
			reads, newModels := 0, 0
			for _, step := range steps {
				if step.Kind == "tool" && step.ToolName == "service_topology" {
					reads++
				}
				if step.Kind == "llm" && step.Model == model && step.Status == "ok" {
					newModels++
				}
			}
			if reads != 1 || newModels == 0 {
				t.Fatal("重复步骤或恢复模型未记录")
			}
			t.Logf("真实终止后恢复成功：模式=%s，步骤=%d，拓扑查询=%d，耗时=%s", mode, len(steps), reads, time.Since(started))
		})
	}
}

// crashPoolCall 固定同一次模型调用的身份，跨进程恢复必须找到原审批单。
func crashPoolCall() llm.ToolCall {
	return llm.ToolCall{ID: "pool-write", Type: "function", Function: llm.Function{Name: "set_db_pool_size", Arguments: `{"service":"fictional","size":30,"reason":"测试回复丢失","evidence_steps":[1],"expected_effect":"验证恢复","risk":"只修改测试服务"}`}}
}

// crashPoolBackend 只连接父测试创建的本机 HTTP 服务，不修改靶场进程。
func crashPoolBackend(t *testing.T, address string) *approval.Backend {
	t.Helper()
	backend, err := approval.NewBackend("", nil, nil, approval.OpsConfig{Services: map[string]string{"fictional": address}, Token: strings.Repeat("a", 32)}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	return backend
}

// TestIntegrationWriteCrash 把故障放在“远端成功、本地尚无结果”的间隙，恢复只能先确认。
func TestIntegrationWriteCrash(t *testing.T) {
	repository, target := integrationStore(t)
	// 测试仓储固定两秒租约，父进程等数据库自然判定过期。
	var mu sync.Mutex
	state := approval.PoolState{Instance: "test-process", Initial: 20, Current: 20}
	posts, gets := 0, 0
	applied := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body struct {
				Operation string `json:"operation"`
				Size      int    `json:"size"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Error("请求不是 JSON")
				return
			}
			mu.Lock()
			posts++
			state.Current, state.Revision, state.LastOperation = body.Size, 1, body.Operation
			mu.Unlock()
			applied <- true
			// 成功已经发生，故意不回包。父进程随即杀死调用方，连接断开后处理器退出。
			<-r.Context().Done()
			return
		}
		mu.Lock()
		gets++
		copy := state
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(copy)
	}))
	defer server.Close()
	backend := crashPoolBackend(t, server.URL)
	service := approval.New(target, backend, repository, llm.NewRedactor())
	ctx := llm.WithLease(t.Context(), "prepare-owner")
	id, err := repository.Manual(ctx, target, "写操作崩溃验收")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := repository.StartTask(ctx, id, "mock", "test", "mock"); err != nil || !ok {
		t.Fatal("准备未取得租约")
	}
	waiting, _, err := service.Handle(ctx, id, 1, crashPoolCall(), map[int]bool{1: true})
	if err != nil || !waiting {
		t.Fatal("没有保存审批")
	}
	if err = repository.EndTask(ctx, id, run.WaitingApproval, agent.Result{}, "等待"); err != nil {
		t.Fatal(err)
	}
	items, err := repository.Approvals(ctx, id)
	if err != nil || len(items) != 1 {
		t.Fatal("没有唯一审批单")
	}
	// 审批来自人的新 HTTP 请求，不能携带已经释放的执行租约。
	if ok, err := repository.DecideApproval(t.Context(), items[0].ID, "test-user", true); err != nil || !ok {
		t.Fatalf("批准失败：%v", err)
	}
	started := time.Now()
	killCrashWorker(t, "write", id, target, applied, "OPSDOCTOR_TEST_OPS_URL="+server.URL)
	// 等真实数据库租约自然过期；不修改断点或远端状态来帮助恢复。
	next := llm.WithLease(t.Context(), "new-owner")
	deadline := time.Now().Add(5 * time.Second)
	for {
		ok, err := repository.StartTask(next, id, "mock", "test", "mock")
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("旧租约没有到期")
		}
		time.Sleep(20 * time.Millisecond)
	}
	waiting, out, err := service.Handle(next, id, 1, crashPoolCall(), map[int]bool{1: true})
	if err != nil || waiting || out.Status != "ok" {
		t.Fatalf("恢复失败：%v %+v", err, out)
	}
	saved, err := repository.FindApproval(next, id, items[0].ID)
	if err != nil || saved.Status != "executed" {
		t.Fatal("恢复结果未保存")
	}
	mu.Lock()
	defer mu.Unlock()
	if posts != 1 || gets != 3 || state.Current != 30 {
		t.Fatalf("GET=%d POST=%d 当前值=%d", gets, posts, state.Current)
	}
	t.Logf("真实终止写执行进程后，GET=%d POST=%d，原单executed，耗时=%s", gets, posts, time.Since(started))
}
