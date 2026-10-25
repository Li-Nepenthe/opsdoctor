// 本文件：通过本机控制接口改变网络延迟和请求流量。
//
// Toxiproxy 转发真实 TCP 数据，增加的等待会自然出现在调用链中。
// 控制请求本身不进入业务日志，也不向服务发送场景编号或答案。
//
// 主要内容：
//   - request：发送有大小限制的 JSON 控制请求。
//   - latency：添加或移除固定名称的网络规则。
//   - load：更新目标 QPS 或热点商品。
//
// 阅读建议：区分网络规则的 upstream/downstream，与请求总体的调用方向。

package action

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
)

// request 不自动重试写控制请求；超时后交给恢复路径处理结果不确定的情况。
func (runner *Runner) request(ctx context.Context, method, url string, input, output any) error {
	// 输入先编码为 JSON；URL 和方法由内部动作选择，不执行文本命令。
	var body []byte
	var err error
	if input != nil {
		body, err = json.Marshal(input)
		if err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := runner.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("访问控制接口：%w", err)
	}
	defer response.Body.Close()
	if method == http.MethodDelete && response.StatusCode == http.StatusNotFound {
		// 重复恢复时规则可能已经被删除，目标状态既然已达到，就不报失败。
		return nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("控制接口返回 HTTP %d", response.StatusCode)
	}
	if output == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return err
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output)
}

// latency 使用固定名称便于重复恢复。只移除本工具的规则，不调用会清空所有代理的 reset。
func (runner *Runner) latency(ctx context.Context, step scenario.Step) error {
	base := strings.TrimRight(runner.ProxyURL, "/") + "/proxies/" + step.Proxy + "/toxics"
	if step.Action == "clear_latency" {
		return runner.request(ctx, http.MethodDelete, base+"/transport-delay", nil, nil)
	}
	input := map[string]any{"name": "transport-delay", "type": "latency", "stream": step.Stream, "toxicity": 1, "attributes": map[string]int{"latency": step.LatencyMS, "jitter": 0}}
	return runner.request(ctx, http.MethodPost, base, input, nil)
}

// load 只发送明确指定的字段，省略字段不会把现有设置重置为零。
func (runner *Runner) load(ctx context.Context, step scenario.Step) error {
	input := make(map[string]any)
	if step.QPS != nil {
		input["qps"] = *step.QPS
	}
	if step.HotSKUID != nil {
		input["hot_sku_id"] = *step.HotSKUID
	}
	return runner.request(ctx, http.MethodPost, runner.LoadURL+"/control", input, nil)
}
