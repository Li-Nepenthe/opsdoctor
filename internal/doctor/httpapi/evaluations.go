// 本文件：把已经保存的评测批次和结果展示给登录用户。
//
// 页面只能读记录，不能从这里启动实验、改评分或执行写操作。场景答案由宿主机
// 评测器使用，医生的 Agent 和工具不会因为增加结果页面而读到答案。
//
// 主要内容：Evaluations 声明查询能力；evalRuns 列出批次；evalResults 读取一个批次。
// 阅读建议：结果去掉初始原文，只返回用于比较的摘要与评分，避免重复加载长日志。

package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/httpx"
	"github.com/gin-gonic/gin"
	"regexp"
	"time"
)

// Evaluations 不提供修改方法，登录用户也不能借结果页改写失败记录。
type Evaluations interface {
	EvalRuns(context.Context) ([]store.EvalRun, error)
	EvalRun(context.Context, string) (store.EvalRun, error)
	EvalResults(context.Context, string) ([]store.EvalResult, error)
}

// evalRuns 有查询预算；数据库不可用时不显示一张误导人的空表。
func (server *api) evalRuns(ctx *gin.Context) {
	if server.options.Evaluations == nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "评测查询未就绪")
		return
	}
	bounded, cancel := context.WithTimeout(ctx.Request.Context(), 5*time.Second)
	defer cancel()
	values, err := server.options.Evaluations.EvalRuns(bounded)
	if err != nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "无法读取评测批次")
		return
	}
	httpx.Success(ctx, 200, values)
}

// evalResults 先确认批次存在，拼错编号不能悄悄显示为“零份报告”。
func (server *api) evalResults(ctx *gin.Context) {
	id := ctx.Param("id")
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`).MatchString(id) {
		httpx.Failure(ctx, 400, "INVALID_ARGUMENT", "批次编号不合法")
		return
	}
	if server.options.Evaluations == nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "评测查询未就绪")
		return
	}
	bounded, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancel()
	run, err := server.options.Evaluations.EvalRun(bounded, id)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Failure(ctx, 404, "NOT_FOUND", "评测批次不存在")
		return
	}
	if err != nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "无法读取评测批次")
		return
	}
	rows, err := server.options.Evaluations.EvalResults(bounded, id)
	if err != nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "无法读取评测结果")
		return
	}
	// 初始症状在诊断详情里已有审计；表格只需要终态摘要，不再传一遍。
	for index := range rows {
		rows[index].Input = nil
	}
	httpx.Success(ctx, 200, map[string]any{"run": run, "items": rows})
}
