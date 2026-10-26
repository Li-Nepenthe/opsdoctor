// 本文件：为只读 HTTP 数据源固定地址、等待时间和响应大小。
//
// 监控后端也可能超时或返回很大的内容。先在读取入口限制资源，工具层的四千字节
// 输出预算才不会变成“先把无限数据读进内存，再截断”。
//
// 主要内容：
//   - New：校验配置地址，并禁止跟随重定向。
//   - Get、Bytes：只发送 GET，读取有上限的 JSON 或文本。
//
// 阅读建议：先看 New 为什么拒绝重定向，再看 Bytes 怎样限制读取量。

package httpbackend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MaximumBytes 是后端响应的读取上限，与工具最终的 4KiB 输出上限不是一回事。
const MaximumBytes = 2 << 20

// Client 的地址只在构造时写入，工具请求不能更改它。
type Client struct {
	base *url.URL     // 已校验的配置地址，不向工具结果输出。
	http *http.Client // 同时设置客户端超时，防止调用者忘记传 deadline。
}

// New 不接受配置中的账号、查询串或片段；这些内容不属于监控服务的根地址。
func New(address string) (*Client, error) {
	base, err := url.Parse(address)
	if err != nil || base.Hostname() == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Path != "" && base.Path != "/") {
		return nil, fmt.Errorf("数据源地址必须是没有账号和路径的 HTTP(S) 根地址")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxConnsPerHost = 4
	transport.ResponseHeaderTimeout = 5 * time.Second
	return &Client{base: base, http: &http.Client{
		Transport: transport, Timeout: 10 * time.Second,
		// 即使配置的后端返回跳转，也不能借此访问另一个地址。
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

// Get 在有界读取后解码 JSON。解析错误不回显响应正文，避免把后端敏感内容带进错误。
func (client *Client) Get(ctx context.Context, path string, query url.Values, target any) error {
	data, err := client.Bytes(ctx, path, query)
	if err != nil {
		return err
	}
	if json.Unmarshal(data, target) != nil {
		return fmt.Errorf("数据源返回的 JSON 格式不符合约定")
	}
	return nil
}

// Bytes 只允许适配器提供相对 API 路径，不使用会把绝对 URL 当新地址的 ResolveReference。
func (client *Client) Bytes(ctx context.Context, path string, query url.Values) ([]byte, error) {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "?#\\") {
		return nil, fmt.Errorf("数据源 API 路径不合法")
	}
	address := *client.base
	address.Path, address.RawPath, address.RawQuery = path, "", query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("无法创建数据源请求")
	}
	response, err := client.http.Do(request)
	if err != nil {
		// 保留取消分类，但不把包含 URL 的底层错误直接返回给工具。
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("数据源请求未完成")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("数据源返回 HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaximumBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("数据源响应读取失败")
	}
	if len(data) > MaximumBytes {
		return nil, fmt.Errorf("数据源响应超过 2MiB，请缩小查询范围")
	}
	return data, nil
}

// Close 释放空闲连接，测试和停机时都不应把后端连接留给垃圾回收来碰运气。
func (client *Client) Close() { client.http.CloseIdleConnections() }
