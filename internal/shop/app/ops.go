// 本文件：读取内部运维入口配置，在业务进程的指标注册表上登记连接池变更。只有设置 OPS_ADDR 才启动独立监听。

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
	if err := registerPoolMetric(metrics, handler); err != nil {
		return nil, err
	}
	return listenOps(parent, stop, address, handler)
}

func registerPoolMetric(metrics *telemetry.Metrics, handler *ops.Handler) error {
	options := prometheus.GaugeOpts{Name: "db_pool_runtime_changed", Help: "本进程的连接池是否被运行时修改过。"}
	return metrics.Register(prometheus.NewGaugeFunc(options, func() float64 {
		if handler.Modified() {
			return 1
		}
		return 0
	}))
}

func listenOps(parent context.Context, stop context.CancelFunc, address string, handler http.Handler) (func() error, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("监听运维入口：%w", err)
	}
	server := &http.Server{
		Addr: address, Handler: handler, ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout: 5 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 30 * time.Second,
	}
	finished := make(chan error, 1)
	// 监听意外退出也能先交回结果，关闭函数在端口排空后取回它。
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
