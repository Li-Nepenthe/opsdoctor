// 本文件：接收本机对速率、热点商品和查询批次的调整，返回生效设置和实际完成的统计。
// 控制请求只改变后续调度。省略参数保留当前设置，明确传零可以暂停普通流量或解除热点。

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
	QPS          int   `json:"qps"`           // 0 暂停，最大 10000。
	HotSKUID     int64 `json:"hot_sku_id"`    // 0 恢复正常分布，正数同时集中商品查询和下单；只读突发另用 /burst。
	ProductCount int   `json:"product_count"` // 本进程实际使用的商品范围，重建后可由实验工具核对。
	Workers      int   `json:"workers"`       // 本进程固定工作者数；控制接口不能临时改变它。
}

// controlState 用互斥锁保护两个字段一起更新，调度者不会看到半次更新。
type controlState struct {
	mu           sync.Mutex
	settings     controlSettings
	productCount int // 热点不能超过本次配置的已有商品范围。
}

// snapshot 返回值副本，调用方离开锁后不能改写共享配置。
func (control *controlState) snapshot() controlSettings {
	control.mu.Lock()
	defer control.mu.Unlock()
	return control.settings
}

// 只更新调用者给出的字段。零值要生效，因为它分别表示暂停流量或解除热点。
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
func controlRouter(control *controlState, stats *statistics, bursts *burstState, logger *slog.Logger) *gin.Engine {
	router := gin.New()
	router.Use(httpx.RequestIDs(logger), httpx.Recovery(logger), httpx.BodyLimit(1024))
	router.GET("/health", func(ctx *gin.Context) { ctx.String(http.StatusOK, "ok\n") })
	router.GET("/control", func(ctx *gin.Context) {
		httpx.Success(ctx, http.StatusOK, gin.H{"settings": control.snapshot(), "statistics": stats.snapshot(time.Now(), false), "burst": bursts.snapshot()})
	})
	router.POST("/burst", func(ctx *gin.Context) {
		var input struct {
			SKUID    int64 `json:"sku_id"`   // 一批只查这个商品，不影响普通流量的选品分布。
			Requests int   `json:"requests"` // 不能超过启动时配置的固定工作者数。
		}
		if !httpx.BindJSON(ctx, &input) {
			return
		}
		report, err := bursts.submit(input.SKUID, input.Requests)
		if err != nil {
			httpx.Failure(ctx, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		// 这里只确认接收，完成数另查 GET /control，不能先把接收数当成成功数。
		logger.Info("已接收一批商品查询", "batch_id", report.ID, "sku_id", report.SKUID, "requests", report.Requests)
		httpx.Success(ctx, http.StatusAccepted, report)
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
