// 本文件：通过本机控制接口调整速率和热点商品。
//
// POST /control 接收 qps 与 hot_sku_id。GET /control 返回生效设置和真实累计统计。
// 这个接口只调整生成行为，不能改写统计结果或向业务服务注入假响应。
//
// 主要内容：
//   - controlSettings、controlState：保存可变配置，用锁协调控制请求与调度循环。
//   - snapshot、update：读取配置或在检查后更新指定字段。
//   - controlRouter：登记控制与健康检查接口。
//
// 阅读建议：qps=0 是暂停，hot_sku_id=0 是恢复 Zipf 分布。省略字段则保留原值。

package loadgen

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/httpx"
	"github.com/gin-gonic/gin"
)

// controlSettings 是一份可复制的配置快照，JSON 中的名字也是控制接口契约。
type controlSettings struct {
	QPS      int   `json:"qps"`        // 0 暂停，最大 10000。
	HotSKUID int64 `json:"hot_sku_id"` // 0 恢复正常分布，正数指定全部商品请求的目标。
}

// controlState 用互斥锁保护两个字段一起更新，调度者不会看到半次更新。
type controlState struct {
	mu           sync.Mutex      // 保护 settings。
	settings     controlSettings // 当前生效的速率和热点。
	productCount int             // 热点不能超过本次配置的已有商品范围。
}

// snapshot 返回值副本，调用方离开锁后不能改写共享配置。
func (control *controlState) snapshot() controlSettings {
	control.mu.Lock()
	defer control.mu.Unlock()
	return control.settings
}

// update 使用指针区分“字段省略”与“明确设为零”，后者在这里是有效操作。
func (control *controlState) update(qps *int, hot *int64) error {
	// 先检查整次更新，两个字段都合法后再加锁写入，避免只改成功一半。
	if qps == nil && hot == nil {
		return fmt.Errorf("至少指定 qps 或 hot_sku_id")
	}
	if qps != nil && (*qps < 0 || *qps > 10000) {
		return fmt.Errorf("qps 必须在 0..10000 之间")
	}
	if hot != nil && (*hot < 0 || *hot > int64(control.productCount)) {
		return fmt.Errorf("hot_sku_id 必须为 0 或已有商品编号")
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	if qps != nil {
		control.settings.QPS = *qps
	}
	if hot != nil {
		control.settings.HotSKUID = *hot
	}
	return nil
}

// controlRouter 只在专用管理端口安装，Compose 把它映射到宿主机回环地址。
func controlRouter(control *controlState, stats *statistics, logger *slog.Logger) *gin.Engine {
	router := gin.New()
	router.Use(httpx.RequestIDs(logger), httpx.Recovery(logger), httpx.BodyLimit(1024))
	router.GET("/health", func(ctx *gin.Context) { ctx.String(http.StatusOK, "ok\n") })
	router.GET("/control", func(ctx *gin.Context) {
		httpx.Success(ctx, http.StatusOK, gin.H{"settings": control.snapshot(), "statistics": stats.snapshot(time.Now(), false)})
	})
	router.POST("/control", func(ctx *gin.Context) {
		var update struct {
			QPS      *int   `json:"qps"`        // nil 表示调用者没有修改速率。
			HotSKUID *int64 `json:"hot_sku_id"` // nil 保持原热点，0 则解除热点。
		}
		if !httpx.BindJSON(ctx, &update) {
			return
		}
		if err := control.update(update.QPS, update.HotSKUID); err != nil {
			httpx.Failure(ctx, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		// 记录的是生效的控制参数，不能把调度目标当成实际完成 QPS。
		settings := control.snapshot()
		logger.Info("压测参数已更新", "target_qps", settings.QPS, "hot_sku_id", settings.HotSKUID)
		httpx.Success(ctx, http.StatusOK, settings)
	})
	return router
}
