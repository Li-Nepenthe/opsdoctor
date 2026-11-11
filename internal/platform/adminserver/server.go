// 本文件：各服务用独立的管理端口提供健康检查、指标和性能剖析。
// 从 New 看调用方提供的探测和指标怎样挂到管理路由，再看 Shutdown 怎样处理超时的连接。

package adminserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// 健康探测要在写响应的期限内结束，即使依赖不通也能及时给监控返回状态。
const (
	probeTimeout  = 2 * time.Second
	headerTimeout = 3 * time.Second
	writeTimeout  = 35 * time.Second // 默认 CPU 采样持续 30 秒，另外留出写回时间。
	idleTimeout   = 30 * time.Second
)

// New 构造管理服务器，由调用方监听端口。健康探测必须响应取消。只有传入指标注册表时才开放 /metrics。
func New(address string, probe func(context.Context) error, gatherers ...prometheus.Gatherer) *http.Server {
	// 使用独立路由表，让性能剖析入口只在管理端口开放。
	mux := http.NewServeMux()
	if len(gatherers) != 0 {
		mux.Handle("GET /metrics", promhttp.HandlerFor(prometheus.Gatherers(gatherers), promhttp.HandlerOpts{}))
	}
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	mux.HandleFunc("GET /health", func(writer http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), probeTimeout)
		defer cancel()
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if err := probe(ctx); err != nil {
			// 探测错误可能带有连接信息，管理端口只公开是否就绪。
			http.Error(writer, "not ready", http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("ok\n"))
	})
	return &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: headerTimeout, WriteTimeout: writeTimeout, IdleTimeout: idleTimeout}
}

// Shutdown 在给定期限内排空请求。排空失败就关闭剩余连接，并保留排空和关闭两处的错误。
func Shutdown(ctx context.Context, server *http.Server) error {
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("管理端口未正常排空：%w", errors.Join(err, server.Close()))
	}
	return nil
}
