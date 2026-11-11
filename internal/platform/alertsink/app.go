// 本文件：Run 验证配置后启动告警接收器，serve 在进程停机时等待正在处理的通知写完日志。

package alertsink

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/adminserver"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/lifecycle"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/logging"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
)

// shutdownTimeout 覆盖一个小通知的处理时间，不让停机无限等待客户端。
const shutdownTimeout = 5 * time.Second

// sinkConfig 供接收器启动使用。日志只记录监听地址，不能把令牌一起输出。
type sinkConfig struct {
	address string
	token   string
}

// loadConfig 在开放端口前核对地址和令牌，错误描述里不包含令牌内容。
func loadConfig(reader config.Reader) (sinkConfig, error) {
	address, addressErr := reader.String("ALERTSINK_ADDR", ":8091")
	token, tokenErr := reader.String("SHOP_ALERT_TOKEN", "")
	if err := errors.Join(addressErr, tokenErr); err != nil {
		return sinkConfig{}, err
	}
	if len(token) < 32 {
		return sinkConfig{}, fmt.Errorf("SHOP_ALERT_TOKEN 至少需要 32 字节")
	}
	return sinkConfig{address: address, token: token}, nil
}

// Run 把通知入口和健康检查挂到同一个端口，漏配令牌时会在监听前退出。
func Run(parent context.Context) error {
	settings, err := loadConfig(config.New())
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout, "alertsink", slog.LevelInfo)
	metrics := telemetry.NewMetrics("alertsink")
	server := adminserver.New(settings.address, func(context.Context) error { return nil }, metrics.Registry)
	mux := http.NewServeMux()
	mux.Handle("/", server.Handler)
	mux.Handle("POST /api/v1/alerts", Handler(settings.token, logger))
	server.Handler = mux
	server.ReadTimeout = shutdownTimeout // 大正文由 Handler 限制，慢速发送还需要时间边界。
	ctx, stop := lifecycle.Notify(parent)
	defer stop()
	logger.Info("临时告警接收器已启动", "version", telemetry.Version, "address", settings.address)
	return serve(ctx, server)
}

// serve 先关闭监听、再取回服务的退出结果。正常关停产生的 ErrServerClosed 不计作故障。
func serve(ctx context.Context, server *http.Server) error {
	finished := make(chan error, 1)
	go func() { finished <- server.ListenAndServe() }()
	select {
	case err := <-finished:
		return err
	case <-ctx.Done():
		// 收到信号只停止接新请求，已有通知仍有五秒完成日志写出。
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	shutdownErr := adminserver.Shutdown(cleanup, server)
	serveErr := <-finished
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(shutdownErr, serveErr)
}
