// 本文件：为三个业务进程装配独立的运维监听器。
//
// 运维接口不能混到公开业务端口。Compose 只在内部网络暴露这个监听器；没有配置
// OPS_ADDR 时不启动，便于本机单独调试旧的业务入口。
//
// 主要内容：StartOps 读取令牌、先绑定端口，再返回必须调用的关闭函数。
// 阅读建议：服务器意外退出会取消业务进程；正常退出则先排空，再读回 Serve 的结果。

package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/adminserver"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/ops"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/product"
	"github.com/prometheus/client_golang/prometheus"
)

// StartOps 借用业务连接池，不创建高权限连接；商品缓存只由订单服务提供。
func StartOps(parent context.Context, stop context.CancelFunc, database *sql.DB, cache product.Cache, metrics *telemetry.Metrics) (func() error, error) {
	reader := config.New()
	address, addressErr := reader.String("OPS_ADDR", "")
	if address == "" && addressErr == nil {
		return func() error { return nil }, nil
	}
	token, tokenErr := reader.String("OPS_TOKEN", "")
	if err := errors.Join(addressErr, tokenErr); err != nil {
		return nil, err
	}
	var products product.Repository
	if cache != nil {
		products = product.NewMySQLRepository(database)
	}
	handler, err := ops.New(token, database, products, cache)
	if err != nil {
		return nil, err
	}
	// 沿用进程的统一注册表与 service 标签，实验工具不需要访问内部写端口。
	if err = metrics.Register(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "db_pool_runtime_changed", Help: "本进程的连接池是否被运行时修改过。"}, func() float64 {
		if handler.Modified() {
			return 1
		}
		return 0
	})); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("监听运维入口：%w", err)
	}
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 30 * time.Second}
	finished := make(chan error, 1)
	// 容量一让 Serve 无论何时结束都能交回结果；关闭函数会读齐并归还 goroutine。
	go func() {
		err := server.Serve(listener)
		if !errors.Is(err, http.ErrServerClosed) {
			stop()
		}
		finished <- err
	}()
	return func() error {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
		defer cancel()
		err := adminserver.Shutdown(cleanup, server)
		serveErr := <-finished
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		return errors.Join(err, serveErr)
	}, nil
}
