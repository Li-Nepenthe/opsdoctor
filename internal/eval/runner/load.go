// 本文件：分别核对正常流量和场景注入完成后的流量，避免把故障本身当作基线漂移。
//
// 主要内容：diagnosticLoad 按顺序解释场景的流量动作；scenarioLoad 从冻结文件取期望值；
// readLoad 读取实际设置；checkLoad 比较四项；checkWorkload 检查全程不变的两项；guardLoad 持续复核。
// 阅读建议：这里要求数值完全相等。场景没有改商品数和工作者数的权限。

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// loadSettings 是四项同时生效的约束，不只检查某一项是否落在宽泛范围内。
type loadSettings struct {
	QPS      int   `json:"qps"`           // 每秒计划发送的请求数，不是历史平均吞吐。
	HotSKU   int64 `json:"hot_sku_id"`    // 零表示分散请求；非零值必须正好匹配场景。
	Products int   `json:"product_count"` // 活跃工作集不能在故障期间悄悄扩大。
	Workers  int   `json:"workers"`       // 工作者数全程保持基线。
}

// normalLoad 同时用于注入前和恢复后，不能因某个场景改过流量就放宽正常状态。
func normalLoad() loadSettings { return loadSettings{QPS: 200, Products: 10000, Workers: 64} }

// diagnosticLoad 模拟动作依次生效的顺序；最后一次 load 才是开始诊断时应有的设置。
func diagnosticLoad(value scenario.Scenario) loadSettings {
	want := normalLoad()
	for _, action := range value.Inject {
		if action.Action == "load" {
			// 空指针表示动作没有修改这一项；显式0才表示停流量或取消热点。
			if action.QPS != nil {
				want.QPS = *action.QPS
			}
			if action.HotSKUID != nil {
				want.HotSKU = *action.HotSKUID
			}
		}
	}
	return want
}

// scenarioLoad 读取受版本哈希保护的场景文件，不相信可被手工编辑的恢复文件中的期望值。
func scenarioLoad(name string) (loadSettings, error) {
	values, err := scenario.Load("eval/scenarios")
	if err != nil {
		return loadSettings{}, err
	}
	for _, value := range values {
		if value.ID == name {
			return diagnosticLoad(value), nil
		}
	}
	return loadSettings{}, fmt.Errorf("冻结场景文件中没有 %s", name)
}

// readLoad 读运行中的控制接口，配置文件里的默认值不能替代实际值。
func readLoad(ctx context.Context, reader config.Reader) (loadSettings, error) {
	base, err := reader.String("LOADGEN_CONTROL_URL", "http://127.0.0.1:6070")
	if err != nil {
		return loadSettings{}, err
	}
	request, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(base, "/")+"/control", nil)
	if err != nil {
		return loadSettings{}, fmt.Errorf("loadgen 控制地址不合法")
	}
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return loadSettings{}, fmt.Errorf("无法读取 loadgen 活跃配置")
	}
	defer response.Body.Close()
	// 指针用于区分“热点为0”和“接口没返回热点字段”，缺字段不能偷偷补成基线。
	var value struct {
		Data struct {
			Settings struct {
				QPS      *int   `json:"qps"`
				HotSKU   *int64 `json:"hot_sku_id"`
				Products *int   `json:"product_count"`
				Workers  *int   `json:"workers"`
			} `json:"settings"`
		} `json:"data"`
	}
	if response.StatusCode != 200 || json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&value) != nil {
		return loadSettings{}, fmt.Errorf("loadgen 没有返回有效配置")
	}
	s := value.Data.Settings
	if s.QPS == nil || s.HotSKU == nil || s.Products == nil || s.Workers == nil {
		return loadSettings{}, fmt.Errorf("loadgen 活跃配置缺字段，评测暂停")
	}
	return loadSettings{QPS: *s.QPS, HotSKU: *s.HotSKU, Products: *s.Products, Workers: *s.Workers}, nil
}

// checkWorkload 覆盖注入、验证和恢复的过渡过程；这些动作也没有改工作集与工作者数的权限。
func checkWorkload(ctx context.Context, reader config.Reader) error {
	actual, err := readLoad(ctx, reader)
	if err != nil {
		return err
	}
	want := normalLoad()
	if actual.Products != want.Products || actual.Workers != want.Workers {
		return fmt.Errorf("评测全程要求商品=%d 工作者=%d，实际商品=%d 工作者=%d", want.Products, want.Workers, actual.Products, actual.Workers)
	}
	return nil
}

// checkLoad 在正常阶段与诊断阶段分别使用对应期望，四项必须全部一致。
func checkLoad(ctx context.Context, reader config.Reader, want loadSettings) error {
	actual, err := readLoad(ctx, reader)
	if err != nil {
		return err
	}
	if actual != want {
		return fmt.Errorf("评测流量要求商品=%d QPS=%d 工作者=%d 热点=%d，实际商品=%d QPS=%d 工作者=%d 热点=%d；请核对当前阶段", want.Products, want.QPS, want.Workers, want.HotSKU, actual.Products, actual.QPS, actual.Workers, actual.HotSKU)
	}
	return nil
}

// guardLoad 每两秒复核，漂移时取消当前诊断或评分；不修改压测器来掩盖漂移。
func guardLoad(parent context.Context, check func(context.Context) error) (context.Context, func(), error) {
	if err := check(parent); err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancelCause(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTicker(2 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				if err := check(ctx); err != nil {
					cancel(err)
					return
				}
			}
		}
	}()
	// 等检查协程结束，避免下一轮已经开始，上一轮还在拿旧期望值检查。
	return ctx, func() { cancel(context.Canceled); <-done }, nil
}
