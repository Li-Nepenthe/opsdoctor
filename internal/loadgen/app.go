// 本文件：启动压测进程，管理控制端口、统计输出和退出。
//
// cmd 调用 Run。只有控制端口成功监听后才开始发请求，避免生成器无法被控制却已持续产生流量。
//
// 主要内容：
//   - Run：读取配置，创建连接复用的 HTTP 客户端并协调各循环。
//   - printWindows：每十秒输出真实区间统计。
//   - logReport：统一输出统计口径和一毫秒桶精度说明。
//
// 阅读建议：停止时先取消调度，再等待在途请求，最后输出全程统计。

package loadgen

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/adminserver"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/lifecycle"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/logging"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// 控制请求很小，单独限制它的等待时间，不借用业务压测的超时配置。
const (
	controlRequestTimeout  = 3 * time.Second  // 控制接口不做慢业务，三秒足以收发小 JSON。
	controlShutdownTimeout = 5 * time.Second  // 多给两秒，让已接收的控制请求完成。
	clientDialTimeout      = time.Second      // 本机连接通常很快，失败时及时记入压测结果。
	clientIdleTimeout      = 30 * time.Second // 复用短期连接，同时回收长期空闲连接。
	statisticsInterval     = 10 * time.Second // 计划书规定每十秒输出一次区间结果。
)

// Run 把启动准备排除在统计之外，实际完成数从工作者开始运行时累计。
func Run(parent context.Context, arguments []string, reader config.Reader, output io.Writer) error {
	settings, err := loadConfig(arguments, reader, output)
	if err != nil {
		return err
	}
	logger := logging.New(output, "loadgen", slog.LevelInfo)
	signalContext, stop := lifecycle.Notify(parent)
	defer stop()
	ctx, cancel := context.WithCancel(signalContext)
	defer cancel()
	if settings.Duration > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, settings.Duration)
		defer timeoutCancel()
	}
	listener, err := net.Listen("tcp", settings.ControlAddress)
	if err != nil {
		return err
	}
	transport := &http.Transport{DialContext: (&net.Dialer{Timeout: clientDialTimeout, KeepAlive: clientIdleTimeout}).DialContext,
		MaxIdleConns: settings.Workers, MaxIdleConnsPerHost: settings.Workers, MaxConnsPerHost: settings.Workers, IdleConnTimeout: clientIdleTimeout, ResponseHeaderTimeout: settings.RequestTimeout}
	defer transport.CloseIdleConnections()
	control := &controlState{settings: controlSettings{QPS: settings.QPS}, productCount: settings.ProductCount}
	metrics := telemetry.NewMetrics("loadgen")
	generator := newGenerator(settings, control, &http.Client{Transport: transport}, logger, metrics)
	router := controlRouter(control, generator.stats, logger)
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{})))
	server := &http.Server{Addr: settings.ControlAddress, Handler: router, ReadHeaderTimeout: controlRequestTimeout, ReadTimeout: controlRequestTimeout, WriteTimeout: controlRequestTimeout, IdleTimeout: clientIdleTimeout}
	finished := make(chan error, 2) // HTTP 与调度循环各写一次，主流程收齐。
	go func() { finished <- server.Serve(listener) }()
	go func() { generator.run(ctx); finished <- nil }()
	reportsDone := make(chan struct{})
	go func() { printWindows(ctx, generator.stats, logger); close(reportsDone) }()
	logger.Info("压测已启动", "version", telemetry.Version, "target_qps", settings.QPS, "products", settings.ProductCount, "workers", settings.Workers, "duration", settings.Duration.String(), "zipf_s", zipfShape, "zipf_v", zipfOffset)
	var result error
	remaining := 2
	select {
	case <-ctx.Done():
		logger.Info("停止调度，等待在途压测请求")
	case result = <-finished:
		remaining--
	}
	if errors.Is(result, http.ErrServerClosed) {
		result = nil
	}
	cancel()
	cleanup, cleanupCancel := context.WithTimeout(context.WithoutCancel(parent), controlShutdownTimeout)
	defer cleanupCancel()
	result = errors.Join(result, adminserver.Shutdown(cleanup, server))
	for range remaining {
		err := <-finished
		if !errors.Is(err, http.ErrServerClosed) {
			result = errors.Join(result, err)
		}
	}
	<-reportsDone // 确保十秒窗口不会在最终汇总之后又打印一次。
	logReport(logger, "total", generator.stats.snapshot(time.Now(), false))
	return result
}

// printWindows 每十秒重置一次窗口，总计保存在另一份直方图中，不受这里影响。
func printWindows(ctx context.Context, stats *statistics, logger *slog.Logger) {
	ticker := time.NewTicker(statisticsInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logReport(logger, "window", stats.snapshot(time.Now(), true))
		}
	}
}

// logReport 把统计作为独立 JSON 对象输出，脚本可以读取数值而不必解析一段中文句子。
func logReport(logger *slog.Logger, scope string, value report) {
	logger.Info("压测统计", "scope", scope, "latency_bucket_ms", 1, "statistics", value)
}
