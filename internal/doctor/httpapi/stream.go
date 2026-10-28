// 本文件：把已保存的步骤通过 SSE 发给页面，断线后按编号补回。
//
// SSE 是一条服务器持续发送文字的 HTTP 响应。内存通知只负责叫醒处理器，真正的
// 内容仍从数据库按序读取，因此并行工具的通知即使乱序，也不会漏掉较小的步骤号。
//
// 主要内容：stream 校验并订阅；sendMissing 补发；writeEvent 限制慢连接的写入时间。
// 阅读建议：先订阅再查库；先查库再订阅会留下丢事件的空窗。

package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/run"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/httpx"
	"github.com/gin-gonic/gin"
)

// stream 不把登录令牌放进 URL；EventSource 使用已认证的 HttpOnly Cookie。
func (server *api) stream(ctx *gin.Context) {
	id := ctx.Param("id")
	after, err := strconv.Atoi(ctx.DefaultQuery("after", "0"))
	if header := ctx.GetHeader("Last-Event-ID"); header != "" {
		after, err = strconv.Atoi(header)
	}
	if err != nil || after < 0 || after > 1_000_000 || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(id) {
		httpx.Failure(ctx, 400, "INVALID_ARGUMENT", "任务或事件编号不合法")
		return
	}
	if server.options.Events == nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "事件服务未就绪")
		return
	}
	bounded, cancel := context.WithTimeout(ctx.Request.Context(), 3*time.Second)
	detail, err := server.options.Store.Detail(bounded, id)
	cancel()
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Failure(ctx, 404, "NOT_FOUND", "任务不存在")
		return
	}
	if err != nil {
		httpx.Failure(ctx, 503, "UNAVAILABLE", "无法读取任务")
		return
	}
	if after > detail.LastSeq {
		httpx.Failure(ctx, 400, "INVALID_ARGUMENT", "事件编号超过已保存步骤")
		return
	}
	subscription := server.options.Events.Subscribe(id)
	defer subscription.Close()
	ctx.Header("Content-Type", "text/event-stream; charset=utf-8")
	ctx.Header("X-Accel-Buffering", "no")
	controller := http.NewResponseController(ctx.Writer)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	// 初次补发和后续通知走同一个读取函数；Last-Event-ID 对应最后写出的持久化步骤。
	for {
		terminal, err := server.sendMissing(ctx, id, &after, controller)
		if err != nil || terminal {
			return
		}
		// 连接建立时任务已结束、而客户端已看过最后一步，仍告诉页面停止重连。
		if terminalStatus(detail.Status) {
			body, _ := json.Marshal(map[string]string{"status": detail.Status})
			_ = writeEvent(ctx.Writer, controller, "", "status", body)
			return
		}
		select {
		case <-ctx.Request.Context().Done():
			return
		case _, ok := <-subscription.Events():
			if !ok {
				return
			}
		case <-ticker.C:
			// 长连接也不能越过登录有效期；下一次自动重连会要求重新登录。
			if _, err := server.options.Tokens.Parse(ctx.GetString("login_token")); err != nil {
				return
			}
			if err := writeEvent(ctx.Writer, controller, "", "", nil); err != nil {
				return
			}
		}
	}
}

// sendMissing 每次取一百步，处理期间新提交的步骤也会被下一页或内存通知发现。
func (server *api) sendMissing(ctx *gin.Context, id string, after *int, controller *http.ResponseController) (bool, error) {
	for {
		bounded, cancel := context.WithTimeout(ctx.Request.Context(), 3*time.Second)
		// 内存事件只是提醒，按数据库顺序读才能覆盖乱序发布。
		steps, err := server.options.Store.StepsAfter(bounded, id, *after)
		cancel()
		if err != nil {
			return false, err
		}
		for _, step := range steps {
			if step.Seq <= *after {
				continue
			}
			body, err := json.Marshal(step)
			if err != nil {
				return false, err
			}
			if err = writeEvent(ctx.Writer, controller, strconv.Itoa(step.Seq), "step", body); err != nil {
				return false, err
			}
			*after = step.Seq
			if step.Kind == "system" && terminalStatus(step.Status) {
				return true, nil
			}
		}
		if len(steps) < 100 {
			return false, nil
		}
	}
}

// terminalStatus 的有限列表也用于停止订阅；阶段 7 的审批状态不属于这里。
func terminalStatus(status string) bool {
	return status == run.Succeeded || status == run.Failed || status == run.Cancelled || status == run.BudgetExceeded
}

// writeEvent 每帧最多等待五秒；空 data 写心跳，不占用持久化步骤编号。
func writeEvent(writer http.ResponseWriter, controller *http.ResponseController, id, name string, data []byte) error {
	err := controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	// httptest 的内存 writer 不支持 deadline；真实 net/http 连接支持，Gin 会 Unwrap 到它。
	if len(data) == 0 {
		_, err = io.WriteString(writer, ": heartbeat\n\n")
	} else {
		prefix := ""
		if id != "" {
			prefix = "id: " + id + "\n"
		}
		_, err = fmt.Fprintf(writer, "%sevent: %s\ndata: %s\n\n", prefix, name, data)
	}
	if err != nil {
		return err
	}
	if err = controller.Flush(); err != nil {
		return err
	}
	return nil
}
