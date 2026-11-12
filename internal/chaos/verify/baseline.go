// 本文件：压测器的商品数和工作者数会影响实验观测。
// Healthy 在连续健康检查前后核对正在运行的压测器设置，避免配置变化被误认为业务症状。

package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Baseline 把实际运行参数的核对接到健康检查之前和之后。
type Baseline struct {
	indexes func(context.Context) ([]string, error) // 生产绑定只读元数据，测试可换成确定样例。
	*Client
	loadURL  string
	products int // 预期活跃商品数，不是数据库总商品数。
	workers  int // 预期固定工作者数，决定最大在途请求数。
}

// WithBaseline 设置本次验收预期的商品数和工作者数。
func WithBaseline(client *Client, loadURL string, products, workers int) *Baseline {
	return &Baseline{Client: client, loadURL: strings.TrimRight(loadURL, "/"), products: products, workers: workers}
}

// Healthy 遇到配置不匹配直接返回，不等三分钟，也不开始任何注入动作。
func (baseline *Baseline) Healthy(ctx context.Context) (Result, error) {
	if err := baseline.check(ctx); err != nil {
		return Result{LastProblem: err.Error()}, err
	}
	if err := baseline.checkRepairs(ctx); err != nil {
		return Result{LastProblem: err.Error()}, err
	}
	result, err := baseline.Client.Healthy(ctx)
	if err != nil {
		return result, err
	}
	// 等待连续健康期间还可能有人修改，结束前再核对一次，不把旧快照当结论。
	if err = baseline.checkRepairs(ctx); err != nil {
		result.LastProblem = err.Error()
	}
	return result, err
}

// check 要求控制接口给出实际参数；缺少字段或数值不符都不能继续验收。
func (baseline *Baseline) check(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseline.loadURL+"/control", nil)
	if err != nil {
		return fmt.Errorf("创建基线查询：%w", err)
	}
	response, err := baseline.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("读取 loadgen 活跃参数：%w", err)
	}
	defer response.Body.Close()
	var body struct {
		Code string `json:"code"` // 防止把 HTTP 成功但业务失败的响应当作有效配置。
		Data struct {
			Settings struct {
				Products int `json:"product_count"` // 实际使用的 Zipf 商品范围。
				Workers  int `json:"workers"`       // 实际启动的工作者数。
			} `json:"settings"`
		} `json:"data"`
	}
	// 控制接口只有少量字段；64 KiB 留足统计空间，同时限制异常响应。
	const maximumControlBytes = 64 << 10
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, maximumControlBytes)).Decode(&body) != nil || body.Code != "OK" {
		return fmt.Errorf("loadgen 控制接口未返回有效参数")
	}
	actual := body.Data.Settings
	if actual.Products != baseline.products || actual.Workers != baseline.workers {
		return fmt.Errorf("实验基线不一致：loadgen 活跃商品数=%d、工作者数=%d，要求 %d、%d。请在 .env 设置 LOADGEN_PRODUCT_COUNT=%d 和 LOADGEN_WORKERS=%d，再执行 docker compose --env-file .env -f deploy/docker-compose.yml up -d --build --no-deps loadgen；自定义基线需同步 CHAOS_BASELINE_PRODUCT_COUNT 和 CHAOS_BASELINE_WORKERS", actual.Products, actual.Workers, baseline.products, baseline.workers, baseline.products, baseline.workers)
	}
	return nil
}
