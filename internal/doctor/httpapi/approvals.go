// 本文件：给已登录的人提供审批单查看、批准和拒绝入口。
//
// 模型没有这些接口的工具定义，也拿不到人的登录会话。批准请求只能是空对象，
// 不能顺便替换索引列或连接池值。Cookie 请求仍经过统一的 CSRF 校验。
//
// 主要内容：Approvals 定义持久化接口；listApprovals 展示原单；decideApproval 保存人的决定。
// 阅读建议：接口不直接执行远端修改，保存决定后只唤醒原来的诊断。

package httpapi

import (
	"context"
	"regexp"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/approval"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/httpx"
	"github.com/gin-gonic/gin"
)

// Approvals 只允许读取和决定，没有修改已保存参数的方法。
type Approvals interface {
	Approvals(context.Context, string) ([]approval.Record, error)
	DecideApproval(context.Context, string, string, bool) (bool, error)
}

// listApprovals 只读一个诊断的单子，避免页面轮询扫描全部审批。
func (server *api) listApprovals(ctx *gin.Context) {
	if server.options.Approvals == nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "审批服务未就绪")
		return
	}
	if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(ctx.Param("id")) {
		httpx.Failure(ctx, 400, "INVALID_ARGUMENT", "诊断编号不合法")
		return
	}
	// 页面轮询不能长时间占住仓储连接，读取失败时保留原单供刷新。
	bounded, cancel := context.WithTimeout(ctx.Request.Context(), 3*time.Second)
	defer cancel()
	values, err := server.options.Approvals.Approvals(bounded, ctx.Param("id"))
	if err != nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "无法读取审批单")
		return
	}
	httpx.Success(ctx, 200, values)
}

// decideApproval 的批准人来自签名后的登录身份，正文不能自行指定 decided_by。
func (server *api) decideApproval(ctx *gin.Context, approved bool) {
	if server.options.Approvals == nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "审批服务未就绪")
		return
	}
	if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(ctx.Param("id")) {
		httpx.Failure(ctx, 400, "INVALID_ARGUMENT", "审批编号不合法")
		return
	}
	// 空对象只表示人的决定。执行参数只能从先前保存的审批单读取。
	var input map[string]any
	if !httpx.BindJSON(ctx, &input) {
		return
	}
	if input == nil || len(input) != 0 {
		httpx.Failure(ctx, 400, "INVALID_ARGUMENT", "审批只接受空对象，不能更改参数")
		return
	}
	bounded, cancel := context.WithTimeout(ctx.Request.Context(), 3*time.Second)
	defer cancel()
	changed, err := server.options.Approvals.DecideApproval(bounded, ctx.Param("id"), ctx.GetString("login_user"), approved)
	if err != nil || !changed {
		httpx.Failure(ctx, 409, "CANNOT_DECIDE", "审批已处理、已过期或诊断已取消，请刷新确认")
		return
	}
	if server.options.Runner != nil {
		server.options.Runner.Wake()
	}
	httpx.Success(ctx, 200, map[string]bool{"decided": true})
}
