// 本文件：实现登录、告警接收、就绪检查和任务查询的 HTTP 细节。
//
// 这里把解析失败与后端失败分开。告警落库失败返回 503，让 Alertmanager 能重试；
// 重复通知是否多建任务，由数据库唯一约束保证。
//
// 主要内容：
//   - login：验证邮箱和 bcrypt 摘要，再签发 JWT。
//   - receiveAlert：先验该系统令牌，再读通知并等待保存成功。
//   - ready、listDiagnoses、diagnosis：有时间边界的只读查询。
//
// 阅读建议：先看 receiveAlert 的验证顺序，再看错误响应为什么不使用 err.Error()。

package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/alerts"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/auth"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/httpx"
	"github.com/gin-gonic/gin"
)

// login 对未知用户也执行一次摘要比较，避免用响应快慢直接枚举邮箱。
func (server *api) login(ctx *gin.Context) {
	if !server.loginLimit.Allow() {
		httpx.Failure(ctx, 429, "RATE_LIMITED", "登录尝试过多，请稍后重试")
		return
	}
	var input struct {
		Email    string `json:"email"`    // 只用于登录查找，不接受展示名。
		Password string `json:"password"` // 只在这次请求中使用，禁止记录。
	}
	if !httpx.BindJSON(ctx, &input) {
		return
	}
	email, err := auth.NormalizeEmail(input.Email)
	if err != nil {
		httpx.Failure(ctx, 401, "UNAUTHORIZED", "邮箱或密码不正确")
		return
	}
	bounded, cancel := context.WithTimeout(ctx.Request.Context(), 3*time.Second)
	defer cancel()
	user, err := server.options.Store.UserByEmail(bounded, email)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "登录服务暂不可用")
		return
	}
	hash := user.PasswordHash
	if errors.Is(err, sql.ErrNoRows) {
		hash = server.options.DummyPasswordHash
	}
	valid := auth.VerifyPassword(hash, input.Password)
	if !valid || err != nil || user.Status != "active" {
		httpx.Failure(ctx, 401, "UNAUTHORIZED", "邮箱或密码不正确")
		return
	}
	token, err := server.options.Tokens.Issue(user.ID)
	if err != nil {
		httpx.Failure(ctx, 500, "INTERNAL", "无法签发登录凭据")
		return
	}
	httpx.Success(ctx, 200, map[string]any{"access_token": token, "token_type": "Bearer", "expires_in": 3600})
}

// receiveAlert 的 target 来自路径并通过令牌验证，不接受正文中另一个系统名作为覆盖。
func (server *api) receiveAlert(ctx *gin.Context) {
	system, ok := server.options.Targets.Get(ctx.Param("target"))
	header := ctx.GetHeader("Authorization")
	if !ok || !strings.HasPrefix(header, "Bearer ") || !system.Authenticate(strings.TrimPrefix(header, "Bearer ")) {
		httpx.Failure(ctx, 401, "UNAUTHORIZED", "告警推送身份无效")
		return
	}
	var value alerts.Notification
	decoder := json.NewDecoder(ctx.Request.Body)
	if err := decoder.Decode(&value); err != nil {
		httpx.Failure(ctx, 400, "INVALID_ARGUMENT", "告警正文格式不合法")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		httpx.Failure(ctx, 400, "INVALID_ARGUMENT", "只能推送一个通知对象")
		return
	}
	// Alertmanager 会带额外的展示字段，允许忽略；真正使用的字段由 alerts 再限制。
	receipt, err := server.notifications.Receive(ctx.Request.Context(), system.Name(), value)
	var invalid alerts.Invalid
	switch {
	case errors.As(err, &invalid):
		httpx.Failure(ctx, 400, "INVALID_ARGUMENT", "告警字段不合法")
	case err != nil:
		httpx.Failure(ctx, 503, "UNAVAILABLE", "告警尚未保存，请重试")
	default:
		httpx.Success(ctx, http.StatusAccepted, receipt)
	}
}

// ready 只检查医生自己的持久化依赖，靶场正在故障不能让接收告警的医生也变成不可用。
func (server *api) ready(ctx *gin.Context) {
	bounded, cancel := context.WithTimeout(ctx.Request.Context(), 2*time.Second)
	defer cancel()
	if err := server.options.Ready(bounded); err != nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "医生数据库未就绪")
		return
	}
	httpx.Success(ctx, 200, map[string]string{"status": "ok"})
}

// listDiagnoses 限制页长与偏移量；未知系统不能静默返回全部系统的任务。
func (server *api) listDiagnoses(ctx *gin.Context) {
	// 先确认筛选的是已接入系统，再检查分页，避免拼错系统名时读到全部任务。
	targetName := ctx.Query("target")
	if targetName != "" {
		if _, ok := server.options.Targets.Get(targetName); !ok {
			httpx.Failure(ctx, 400, "INVALID_ARGUMENT", "未知系统")
			return
		}
	}
	status := ctx.Query("status")
	if status != "" && status != "pending" {
		httpx.Failure(ctx, 400, "INVALID_ARGUMENT", "本阶段只支持 pending 状态")
		return
	}
	page, err := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 1000 {
		httpx.Failure(ctx, 400, "INVALID_ARGUMENT", "页码不合法")
		return
	}
	size, err := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	if err != nil || size < 1 || size > 100 {
		httpx.Failure(ctx, 400, "INVALID_ARGUMENT", "页长不合法")
		return
	}
	bounded, cancel := context.WithTimeout(ctx.Request.Context(), 3*time.Second)
	defer cancel()
	values, err := server.options.Store.List(bounded, targetName, status, size, (page-1)*size)
	if err != nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "无法读取任务")
		return
	}
	httpx.Success(ctx, 200, map[string]any{"items": values, "page": page, "page_size": size})
}

// diagnosis 只接受本服务生成的 ID，查询失败时不泄露 SQL 或数据库连接细节。
func (server *api) diagnosis(ctx *gin.Context) {
	// ID 只是查询条件。数据库错误只返回固定提示，不能把底层 SQL 带到 HTTP 响应。
	id := ctx.Param("id")
	if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(id) {
		httpx.Failure(ctx, 400, "INVALID_ARGUMENT", "任务编号不合法")
		return
	}
	bounded, cancel := context.WithTimeout(ctx.Request.Context(), 3*time.Second)
	defer cancel()
	value, err := server.options.Store.Detail(bounded, id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		httpx.Failure(ctx, 404, "NOT_FOUND", "任务不存在")
	case err != nil:
		httpx.Failure(ctx, 503, "UNAVAILABLE", "无法读取任务")
	default:
		httpx.Success(ctx, 200, value)
	}
}
