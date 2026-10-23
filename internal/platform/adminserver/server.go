// 本文件：管理端口的健康检查和有界关闭。
//
// 服务 app 注入依赖探测，New 只负责把探测变成 /health 的状态码。这样管理基础设施无需知道库存或订单。
// 先看探测预算，再看 Shutdown 失败时为什么还要 Close。
// 指标与 pprof 只挂在这张独立的管理路由表上，业务端口不会跟着开放它们。
//
// 主要内容：
//   - New：创建未启动的 HTTP 服务器。probe 由 app 注入（例如数据库 Ping），本包不认识业务库。
//   - Shutdown：在正常排空失败后主动 Close，避免已经超过停机预算的连接继续占用资源。

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

// 管理请求应很短。探测预算小于写响应预算，防止数据库不可用时占住 HTTP 连接。
const (
	probeTimeout  = 2 * time.Second
	headerTimeout = 3 * time.Second
	writeTimeout  = 35 * time.Second // pprof 默认 CPU 采样需要 30 秒，另外留出写回时间。
	idleTimeout   = 30 * time.Second
)

// New 创建未启动的 HTTP 服务器。probe 由 app 注入（例如数据库 Ping），本包不认识业务库。
// probe 必须响应 context，不能自行启动无限运行的 goroutine。调用方负责监听、Serve 和 Shutdown。
func New(address string, probe func(context.Context) error, gatherers ...prometheus.Gatherer) *http.Server {
	mux := http.NewServeMux()
	// 显式注册，绝不把 DefaultServeMux 整体交给业务服务。零个 gatherer 供原有健康测试使用。
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
			// 只返回健康与否，不能把数据库错误和连接信息暴露到管理端口。
			http.Error(writer, "not ready", http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusOK)
		// 健康状态已经通过状态码给出。客户端提前断连时无法再回复，写出错误无需重试。
		_, _ = writer.Write([]byte("ok\n"))
	})
	return &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: headerTimeout, WriteTimeout: writeTimeout, IdleTimeout: idleTimeout}
}

// Shutdown 在正常排空失败后主动 Close，避免已经超过停机预算的连接继续占用资源。
func Shutdown(ctx context.Context, server *http.Server) error {
	if err := server.Shutdown(ctx); err != nil {
		// errors.Join 保留超时和强制关闭两个步骤的错误。Close 成功返回的 nil 会被忽略。
		return fmt.Errorf("管理端口未正常排空：%w", errors.Join(err, server.Close()))
	}
	return nil
}
