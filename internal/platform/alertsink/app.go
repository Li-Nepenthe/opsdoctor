// 本文件：启动临时告警接收器，并在退出时排空已接收的请求。
//
// cmd/alertsink 只调用 Run。这里使用独立的注册表和管理路由，不接入医生的任何业务包。
//
// 主要内容：
//   - Run：读取监听地址和令牌，创建日志、指标与 HTTP 服务。
//   - serve：等待停止信号或监听错误，给在途通知有限的收尾时间。
//
// 阅读建议：令牌是必填配置。启动日志只写版本和地址，不能打印整个配置。

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

// Run 先验证令牌再开放端口，漏配环境变量时不能意外启动一个无认证接收器。
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

// serve 的结果通道只有一次写入，主流程退出前一定读到它，不遗留监听 goroutine。
func serve(ctx context.Context, server *http.Server) error {
	finished := make(chan error, 1) // HTTP 循环写，主流程读；无需关闭这个一次性通道。
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
