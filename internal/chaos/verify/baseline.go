// 本文件：在等待健康指标之前，核对压测器实际使用的实验参数。
//
// 相同 QPS 下，五十万商品和一万商品的缓存未命中率不同。只看告警会把参数变化
// 误当成故障，所以 healthy 和 selftest 要先向正在运行的 loadgen 取一份配置。
//
// 主要内容：
//   - Baseline、WithBaseline：把预期商品数和工作者数交给健康检查。
//   - Healthy、check：不一致时立即失败，给出重新创建 loadgen 的办法。
//
// 阅读建议：这些值来自控制接口，不是宿主机的 LOADGEN 环境变量。

package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Baseline 在原有指标观察前加参数核对。嵌入 Client 仍提供原来的 Symptom 方法。
type Baseline struct {
	indexes  func(context.Context) ([]string, error) // 生产绑定只读元数据，测试可换成确定样例。
	*Client                                          // 负责指标、告警和连续健康时间。
	loadURL  string                                  // 压测器控制根地址，仅供宿主机实验使用。
	products int                                     // 预期活跃商品数，不是数据库总商品数。
	workers  int                                     // 预期固定工作者数，决定最大在途请求数。
}

// WithBaseline 由 app 传入校验过的配置，单元测试仍可单独验证原来的指标观察器。
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

// check 只读取小型配置响应；老版本缺字段时得到零值，也必须报错，不能默认放行。
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
