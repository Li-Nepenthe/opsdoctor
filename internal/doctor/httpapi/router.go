// 本文件：把登录、系统工具和告警入口接到同一个 Gin 服务。
//
// 人通过 JWT 登录后选择系统；Alertmanager 则只持有一个系统的推送令牌。
// 两种身份不混用：推送令牌不能执行工具，登录令牌也不能冒充告警来源。
//
// 主要内容：
//   - Repository、Options：列出接口层实际需要的依赖。
//   - New：安装通用保护，区分公开健康检查、登录、通知和已登录操作。
//   - requireLogin、listTargets、runTool：校验身份并调用已绑定的注册表。
//
// 阅读建议：先看路由分组，确认哪一条入口经过哪一种鉴权。

package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/alerts"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/auth"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/store"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/target"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/httpx"
	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// Repository 定义在接口使用方，测试可以观察持久化是否被调用，而不启动 MySQL。
type Repository interface {
	alerts.Receiver
	UserByEmail(context.Context, string) (store.User, error)
	List(context.Context, string, string, int, int) ([]store.Diagnosis, error)
	Detail(context.Context, string) (store.Detail, error)
}

// Options 只接收已经装配的对象，不读取环境变量或创建第二套数据库池。
type Options struct {
	Targets           *target.Catalog             // 各系统独立的工具与告警身份。
	Store             Repository                  // 医生自己的用户和任务。
	Tokens            *auth.Tokens                // JWT 签发与验证。
	Logger            *slog.Logger                // 日志只记录固定字段，不记录请求正文。
	Ready             func(context.Context) error // 检查医生自己的数据库。
	Metrics           http.Handler                // 通用进程指标，不在这里创建全局注册表。
	DummyPasswordHash string                      // 未知邮箱也做同等成本的密码比较。
}

// api 持有服务范围的登录限流器，不能让猜密码请求无限占用 bcrypt 的 CPU。
type api struct {
	options       Options         // 构造后保持不变。
	notifications *alerts.Service // 所有告警共用格式校验，但身份由路由绑定。
	loginLimit    *rate.Limiter   // 单实例每秒最多两次登录尝试，允许短时四次。
}

// New 不挂载页面或 Agent 路由，阶段 5 只开放用户手动取证需要的接口。
func New(options Options) *gin.Engine {
	server := &api{options: options, notifications: alerts.New(options.Store), loginLimit: rate.NewLimiter(2, 4)}
	router := gin.New()
	router.Use(httpx.RequestIDs(options.Logger), httpx.Recovery(options.Logger), httpx.BodyLimit(httpx.DefaultBodyLimit))
	router.GET("/health/live", func(ctx *gin.Context) { httpx.Success(ctx, http.StatusOK, map[string]string{"status": "ok"}) })
	router.GET("/health/ready", server.ready)
	router.GET("/metrics", gin.WrapH(options.Metrics))
	router.POST("/api/v1/auth/login", server.login)
	router.POST("/api/v1/targets/:target/alerts", server.receiveAlert)
	// JWT 只保护人的入口；告警推送必须走它自己的系统令牌校验。
	secured := router.Group("/api/v1", server.requireLogin)
	secured.GET("/targets", server.listTargets)
	secured.POST("/targets/:target/tools/:tool/run", server.runTool)
	secured.GET("/diagnoses", server.listDiagnoses)
	secured.GET("/diagnoses/:id", server.diagnosis)
	return router
}

// requireLogin 不把解析失败的细节回显给客户端，令牌原文也不会写入日志。
func (server *api) requireLogin(ctx *gin.Context) {
	raw := ctx.GetHeader("Authorization")
	if !strings.HasPrefix(raw, "Bearer ") {
		httpx.Failure(ctx, 401, "UNAUTHORIZED", "请先登录")
		return
	}
	if _, err := server.options.Tokens.Parse(strings.TrimPrefix(raw, "Bearer ")); err != nil {
		httpx.Failure(ctx, 401, "UNAUTHORIZED", "登录凭据无效或已过期")
		return
	}
	ctx.Next()
}

// listTargets 只返回名称和工具说明，不把接入地址、账号变量或令牌发给客户端。
func (server *api) listTargets(ctx *gin.Context) {
	type summary struct {
		Name  string             `json:"name"`
		Tools []tools.Definition `json:"tools"`
	} // 每个系统都有自己的工具菜单。
	values := []summary{}
	for _, name := range server.options.Targets.Names() {
		system, _ := server.options.Targets.Get(name)
		values = append(values, summary{name, system.Tools().List()})
	}
	httpx.Success(ctx, 200, values)
}

// runTool 是 doctorctl 与未来其他入口共用的调用路径，不能直接调用适配器。
func (server *api) runTool(ctx *gin.Context) {
	system, ok := server.options.Targets.Get(ctx.Param("target"))
	if !ok {
		httpx.Failure(ctx, 404, "TARGET_NOT_FOUND", "系统不存在")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 8192))
	if err != nil {
		httpx.Failure(ctx, 400, "INVALID_ARGUMENT", "工具参数超过读取上限")
		return
	}
	// 校验错误也作为工具结果返回。调用者按 status 判断，不能只看 HTTP 200。
	result := system.Tools().Run(ctx.Request.Context(), ctx.Param("tool"), raw)
	httpx.Success(ctx, 200, result)
}
