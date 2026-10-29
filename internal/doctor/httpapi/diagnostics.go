// 本文件：接收手动症状、取消任务，并向页面说明当前执行模式和预算。
//
// 页面只能选择已经接入的系统，不能在正文里提供工具地址。手动任务与告警任务
// 都进同一队列，取消也由同一个调度器传播到模型与工具。
//
// 主要内容：manual 创建任务；cancelDiagnosis 发取消信号；runtimeStatus 显示模式和费用。
// 阅读建议：这些接口由 requireLogin 保护，Cookie 的 POST 还要通过 CSRF 校验。

package httpapi

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/httpx"
	"github.com/gin-gonic/gin"
)

// manual 限制症状长度，服务端绑定系统，保存后才唤醒调度器。
func (server *api) manual(ctx *gin.Context) {
	var input struct {
		Target   string `json:"target"`
		Symptoms string `json:"symptoms"`
	}
	if !httpx.BindJSON(ctx, &input) {
		return
	}
	if _, ok := server.options.Targets.Get(input.Target); !ok || strings.TrimSpace(input.Symptoms) == "" || len(input.Symptoms) > 8192 {
		httpx.Failure(ctx, 400, "INVALID_ARGUMENT", "请选择已接入系统，并填写 1 至 8192 字节的症状")
		return
	}
	if server.options.Runner == nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "调度器未就绪")
		return
	}
	bounded, cancel := context.WithTimeout(ctx.Request.Context(), 3*time.Second)
	defer cancel()
	status, err := server.options.Runner.Status(bounded)
	if err != nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "无法读取费用账本")
		return
	}
	// 免费脚本仍可演示；真实模型额度不足时不让手动按钮无限堆积新任务。
	if server.options.Provider != "mock" && status.Blocked {
		httpx.Failure(ctx, 409, "BUDGET_EXCEEDED", status.Reason)
		return
	}
	id, err := server.options.Store.Manual(bounded, input.Target, input.Symptoms)
	if err != nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "任务未保存，请重试")
		return
	}
	server.options.Runner.Wake()
	httpx.Success(ctx, http.StatusAccepted, map[string]string{"id": id, "status": "pending"})
}

// cancelDiagnosis 只接受服务生成的 ID，返回成功表示取消已请求，终态随后通过 SSE 到达。
func (server *api) cancelDiagnosis(ctx *gin.Context) {
	// 路径编号只用于定位已创建的任务，不能携带其他执行参数。
	id := ctx.Param("id")
	if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(id) {
		httpx.Failure(ctx, 400, "INVALID_ARGUMENT", "任务编号不合法")
		return
	}
	if server.options.Runner == nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "调度器未就绪")
		return
	}
	bounded, cancel := context.WithTimeout(ctx.Request.Context(), time.Second)
	defer cancel()
	if err := server.options.Runner.Cancel(bounded, id); err != nil {
		httpx.Failure(ctx, 409, "CANNOT_CANCEL", "任务已结束，或取消尚未保存，请刷新确认")
		return
	}
	httpx.Success(ctx, 202, map[string]bool{"cancellation_requested": true})
}

// runtimeStatus 同一个读数同时驱动横幅和按钮状态，费用原因不是页面自己猜出来的。
func (server *api) runtimeStatus(ctx *gin.Context) {
	if server.options.Runner == nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "调度器未就绪")
		return
	}
	bounded, cancel := context.WithTimeout(ctx.Request.Context(), 2*time.Second)
	defer cancel()
	budget, err := server.options.Runner.Status(bounded)
	if err != nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "无法读取费用账本")
		return
	}
	executions := map[string]run.ExecutionStatus{}
	// 评测器必须看运行中的两套配置，不能把本机尚未部署的 .env 当作生效配置。
	if server.options.Executions != nil {
		executions, err = server.options.Executions(bounded)
		if err != nil {
			httpx.Failure(ctx, 503, "UNAVAILABLE", "无法读取评测模型的费用账本")
			return
		}
	}
	httpx.Success(ctx, 200, map[string]any{"provider": server.options.Provider, "model": server.options.Model, "budget": budget, "executions": executions})
}
