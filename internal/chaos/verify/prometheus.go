// 本文件：读取 Prometheus 的查询结果和正在触发的告警。
//
// 验收必须看业务指标，不能看注入器是否已经发出命令。
// 空结果、NaN 和接口错误都不能算成功，否则监控断开反而会显示健康。
//
// 主要内容：
//   - Client、New：准备有超时的只读 HTTP 客户端。
//   - Query：解释即时查询返回的数值。
//   - Alerts：只返回 firing 状态的告警名称。
//
// 阅读建议：PromQL 比较表达式不满足时通常返回空列表，不是返回 false 字符串。

package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client 的间隔和持续时间集中保存，单元测试可缩短时间，不必真的等一分钟。
type Client struct {
	URL    string        // 只读 Prometheus 根地址。
	HTTP   *http.Client  // 每次查询最多五秒，外层 context 还可提前取消。
	poll   time.Duration // 两次采样间隔，不影响 Prometheus 自己的抓取周期。
	stable time.Duration // 健康条件必须连续成立的时间。
}

// New 使用五秒轮询，能在十秒采集周期内及时看到新样本，又不会不断空转。
func New(address string) *Client {
	return &Client{URL: strings.TrimRight(address, "/"), HTTP: &http.Client{Timeout: 5 * time.Second}, poll: 5 * time.Second, stable: time.Minute}
}

// get 限制响应大小，并要求 HTTP 成功；各调用方再检查 Prometheus JSON 中的业务状态。
func (client *Client) get(ctx context.Context, path string, output any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.URL+path, nil)
	if err != nil {
		return err
	}
	response, err := client.HTTP.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("指标接口返回 HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(output)
}

// Query 用普通比较后的样本判断条件；没有样本表示条件尚未出现，不是后端错误。
func (client *Client) Query(ctx context.Context, expression string) (bool, error) {
	var response struct {
		Status string
		Data   struct {
			ResultType string
			Result     []struct{ Value []json.RawMessage }
		}
	}
	if err := client.get(ctx, "/api/v1/query?query="+url.QueryEscape(expression), &response); err != nil {
		return false, err
	}
	if response.Status != "success" || response.Data.ResultType != "vector" {
		return false, fmt.Errorf("指标接口未返回成功的向量查询")
	}
	for _, sample := range response.Data.Result {
		if len(sample.Value) != 2 {
			return false, fmt.Errorf("指标样本格式错误")
		}
		var raw string
		if err := json.Unmarshal(sample.Value[1], &raw); err != nil {
			return false, err
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return false, err
		}
		// PromQL 的普通比较在成立时保留左侧值；零错误率也会保留 0，不能因此判失败。
		if !math.IsNaN(value) && !math.IsInf(value, 0) {
			return true, nil
		}
	}
	return false, nil
}

// Alerts 排除 pending，它表示条件还在观察中，尚未真的触发告警。
func (client *Client) Alerts(ctx context.Context) ([]string, error) {
	// 直接查 Prometheus 的规则状态，不把 Alertmanager 的通知重试当作新告警。
	var response struct {
		Status string
		Data   struct {
			Alerts []struct {
				State  string
				Labels map[string]string
			}
		}
	}
	if err := client.get(ctx, "/api/v1/alerts", &response); err != nil {
		return nil, err
	}
	if response.Status != "success" {
		return nil, fmt.Errorf("告警查询未成功")
	}
	var names []string
	for _, alert := range response.Data.Alerts {
		if alert.State == "firing" {
			names = append(names, alert.Labels["alertname"])
		}
	}
	return names, nil
}
