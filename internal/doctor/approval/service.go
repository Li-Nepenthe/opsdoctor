// 本文件：把一次模型写工具调用接到持久化审批流程。
//
// 同一轮的调用 ID 与诊断 ID 决定唯一审批编号。保存后崩溃再恢复，仍找到原单，
// 不会因为重新走到这一行就多出第二张申请。
//
// 主要内容：New 绑定系统；Handle 处理申请和恢复；result 产生明确的工具结果。
// 阅读建议：只有 ClaimApproval 返回 executing 后，才可能进入真正的写操作。

package approval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
)

// Service 每次从已绑定系统取后端；调用参数不能换系统或指定主机。
type Service struct {
	target     string
	backend    *Backend
	repository Repository
	redactor   *llm.Redactor
}

// New 不联网，后端账号与工具权限都在启动时固定。
func New(target string, backend *Backend, repository Repository, redactor *llm.Redactor) *Service {
	return &Service{target: target, backend: backend, repository: repository, redactor: redactor}
}

// List 只提供当前配置能够实施的写工具，执行仍必须经过数据库审批。
func (service *Service) List() []llm.Tool { return service.backend.definitions() }

// Handles 使用代码中的固定名单，模型不能给普通查看工具加一个“写”属性来改变权限。
func (service *Service) Handles(name string) bool {
	return name == "create_index" || name == "set_db_pool_size" || name == "warm_cache"
}

// Handle 的 waiting=true 表示已经保存申请，Agent 应让出执行线程而不是轮询人有没有批准。
func (service *Service) Handle(ctx context.Context, diagnosis string, parent int, call llm.ToolCall, steps map[int]bool) (bool, tools.Result, error) {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", diagnosis, parent, call.ID)))
	id := hex.EncodeToString(hash[:16])
	record, err := service.repository.FindApproval(ctx, diagnosis, id)
	if errors.Is(err, ErrNotFound) {
		// 准备阶段只有元数据查询，共用只读工具十秒限制；还没有任何写操作。
		bounded, cancel := context.WithTimeout(ctx, tools.Timeout)
		prepared, problem := service.backend.prepare(bounded, id, call.Function.Name, json.RawMessage(call.Function.Arguments), steps)
		cancel()
		if problem != nil {
			return false, result(call.Function.Name, "invalid_argument", problem.Error()), nil
		}
		prepared.ID, prepared.DiagnosisID, prepared.Target, prepared.ParentSeq, prepared.CallID = id, diagnosis, service.target, parent, call.ID
		// 理由、预览与原始参数也可能包含敏感文字，入库前使用同一个脱敏器。
		raw, _ := json.Marshal(prepared)
		raw, err = service.redactor.JSON(raw)
		if err != nil {
			return false, tools.Result{}, err
		}
		if err = json.Unmarshal(raw, &prepared); err != nil {
			return false, tools.Result{}, err
		}
		record, err = service.repository.CreateApproval(ctx, prepared)
	}
	if err != nil {
		return false, tools.Result{}, err
	}
	if record.Target != service.target || record.Tool != call.Function.Name {
		return false, tools.Result{}, fmt.Errorf("审批单与当前系统或工具不符")
	}
	switch record.Status {
	case "pending":
		return true, tools.Result{}, nil
	case "rejected":
		return false, result(record.Tool, "rejected", "审批已拒绝，未执行；请基于已有证据继续"), nil
	case "expired":
		return false, result(record.Tool, "expired", "审批已过期，未执行；请基于已有证据继续"), nil
	case "executed", "failed":
		return false, record.Result, nil
	case "approved", "executing":
		// 领取只使用库中记录。就算模型这次换了参数，也不会传到执行器里。
		record, err = service.repository.ClaimApproval(ctx, diagnosis, id)
		if err != nil {
			return false, tools.Result{}, err
		}
		started := time.Now()
		out := service.backend.execute(ctx, record, func(connection int64) error {
			return service.repository.ApprovalConnection(ctx, diagnosis, id, connection)
		})
		out.DurationMS = time.Since(started).Milliseconds()
		raw, _ := json.Marshal(out)
		clean, cleanErr := service.redactor.JSON(raw)
		if cleanErr != nil {
			return false, tools.Result{}, cleanErr
		}
		_ = json.Unmarshal(clean, &out)
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 500*time.Millisecond)
		defer cancel()
		if err = service.repository.FinishApproval(cleanup, diagnosis, id, out); err != nil {
			return false, tools.Result{}, err
		}
		return false, out, nil
	default:
		return false, tools.Result{}, fmt.Errorf("审批状态不支持执行")
	}
}

// result 只生成固定的小结果，不把数据库错误、后端地址或账号回传给模型。
func result(tool, status, summary string) tools.Result {
	return tools.Result{Tool: tool, Status: status, Summary: summary, Data: json.RawMessage(`null`)}
}
