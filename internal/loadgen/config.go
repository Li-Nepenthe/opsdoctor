// 本文件：读取压测工具的目标地址、速率和运行时长。
//
// Run 先读取环境默认值，再用显式命令行参数覆盖。配置检查先于任何网络请求。
//
// 主要内容：
//   - Config：保存调度、连接与停止相关的参数。
//   - loadConfig：读取环境与 flags，拒绝不可用的目标或过大的资源配置。
//
// 阅读建议：QPS 为 0 表示暂停，Duration 为 0 表示持续运行，这两个零的含义不同。

package loadgen

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// Config 保存已经检查过的参数。目标地址不允许含用户名密码，避免日志与请求意外携带凭据。
type Config struct {
	Target         string        // LOADGEN_TARGET，默认本机订单服务。
	ControlAddress string        // LOADGEN_ADDR，控制端口默认 :6070。
	QPS            int           // LOADGEN_QPS，目标请求速率，默认 200。
	ProductCount   int           // LOADGEN_PRODUCT_COUNT，全量造数默认 500000，small 使用 100。
	Workers        int           // LOADGEN_WORKERS，固定并发上限，默认 64。
	RequestTimeout time.Duration // LOADGEN_REQUEST_TIMEOUT，每次 HTTP 尝试的预算。
	Duration       time.Duration // LOADGEN_DURATION，0 表示一直运行，直到收到退出信号。
}

// loadConfig 把环境作为默认值，命令行覆盖只影响本次进程，不写回 .env。
func loadConfig(arguments []string, reader config.Reader, output io.Writer) (Config, error) {
	target, targetErr := reader.String("LOADGEN_TARGET", "http://127.0.0.1:8081")
	address, addressErr := reader.String("LOADGEN_ADDR", ":6070")
	qps, qpsErr := reader.Int("LOADGEN_QPS", 200, 0, 10000)
	products, productsErr := reader.Int("LOADGEN_PRODUCT_COUNT", 500000, 1, 500000)
	workers, workersErr := reader.Int("LOADGEN_WORKERS", 64, 1, 256)
	timeout, timeoutErr := reader.Duration("LOADGEN_REQUEST_TIMEOUT", 3*time.Second, time.Millisecond, 30*time.Second)
	duration, durationErr := reader.Duration("LOADGEN_DURATION", 0, 0, 24*time.Hour)
	if err := errors.Join(targetErr, addressErr, qpsErr, productsErr, workersErr, timeoutErr, durationErr); err != nil {
		return Config{}, err
	}
	value := Config{Target: target, ControlAddress: address, QPS: qps, ProductCount: products, Workers: workers, RequestTimeout: timeout, Duration: duration}
	flags := flag.NewFlagSet("loadgen", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&value.Target, "target", value.Target, "订单服务的 HTTP 地址")
	flags.StringVar(&value.ControlAddress, "control", value.ControlAddress, "本机控制接口监听地址")
	flags.IntVar(&value.QPS, "qps", value.QPS, "目标 QPS，0 表示暂停")
	flags.IntVar(&value.ProductCount, "products", value.ProductCount, "已有商品数量，small 数据使用 100")
	flags.IntVar(&value.Workers, "workers", value.Workers, "同时发送请求的工作者上限")
	flags.DurationVar(&value.RequestTimeout, "timeout", value.RequestTimeout, "每次 HTTP 尝试的超时")
	flags.DurationVar(&value.Duration, "duration", value.Duration, "运行时长，0 表示持续运行")
	if err := flags.Parse(arguments); err != nil {
		return Config{}, err
	}
	if flags.NArg() != 0 {
		return Config{}, fmt.Errorf("loadgen 不接受位置参数")
	}
	parsed, err := url.Parse(value.Target)
	// 只接受根地址，避免把已有 query 或凭据带到每个生成请求中。
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return Config{}, fmt.Errorf("压测目标必须是无凭据、路径和查询参数的 HTTP 根地址")
	}
	value.Target = strings.TrimRight(value.Target, "/")
	if value.QPS < 0 || value.QPS > 10000 || value.ProductCount <= 0 || value.ProductCount > 500000 || value.Workers <= 0 || value.Workers > 256 || value.RequestTimeout < time.Millisecond || value.RequestTimeout > 30*time.Second || value.Duration < 0 || value.Duration > 24*time.Hour {
		return Config{}, fmt.Errorf("压测参数超出允许范围")
	}
	return value, nil
}
