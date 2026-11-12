// 本文件：监控采样决定实验是否通过，单次健康不足以证明系统恢复。
// Symptom 分别记住症状和相关告警，Healthy 在任一坏样本后重新计时。

package verify

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
)

// Result 即使失败也保留等待时间，不能只在成功时记录实验数据。
type Result struct {
	Elapsed     time.Duration `json:"elapsed"`      // 实际墙钟等待，非配置值。
	SymptomSeen bool          `json:"symptom_seen"` // 至少一次业务指标满足场景条件。
	Alerts      []string      `json:"alerts"`
	LastProblem string        `json:"last_problem,omitempty"`
}

// Symptom 的 ctx 由注入开始时间计算截止，部署耗时也计入三分钟，不另送三分钟。
func (client *Client) Symptom(ctx context.Context, symptom scenario.Symptom) (result Result, err error) {
	// 无故障对照不能等到一条告警才算通过；仍使用完整的连续健康检查。
	if symptom.Healthy {
		return client.Healthy(ctx)
	}
	// 两个信号可能不在同一轮采样出现，分别记住是否真正见过，避免漏掉短暂症状。
	start := time.Now()
	defer func() { result.Elapsed = time.Since(start) }()
	for {
		seen, queryErr := client.Query(ctx, symptom.PromQL)
		if queryErr == nil {
			result.SymptomSeen = result.SymptomSeen || seen
		} else {
			result.LastProblem = queryErr.Error()
		}
		names, alertErr := client.Alerts(ctx)
		if alertErr != nil {
			result.LastProblem = alertErr.Error()
		}
		for _, name := range names {
			if slices.Contains(symptom.Alerts, name) && !slices.Contains(result.Alerts, name) {
				result.Alerts = append(result.Alerts, name)
			}
		}
		if result.SymptomSeen && len(result.Alerts) > 0 {
			return result, nil
		}
		if err := wait(ctx, client.poll); err != nil {
			return result, fmt.Errorf("症状或相关告警未在时限内出现：%w", err)
		}
	}
}

// healthyQueries 不把无流量当健康；三个业务端点和三个依赖采集器都必须可用。
func healthyQueries() []string {
	return []string{
		`sum(rate(http_server_request_duration_seconds_count{service="order-service"}[1m])) > 10`,
		`count(up{job="shop",service=~"order-service|inventory-service|order-worker"} == 1) == 3`,
		`count(up{job="middleware"} == 1) == 3`,
		`max(histogram_quantile(0.99,sum by (le,route,method)(rate(http_server_request_duration_seconds_bucket{service="order-service"}[1m])))) < 1`,
		`(sum(rate(http_server_request_duration_seconds_count{service="order-service",status=~"[45].."}[1m])) or vector(0)) / sum(rate(http_server_request_duration_seconds_count{service="order-service"}[1m])) < 0.01`,
		`sum(kafka_consumergroup_lag{consumergroup="order-worker",topic="order.created"}) < 10`,
		`mysql_up == 1`,
		`redis_up == 1`,
	}
}

// healthSample 任一查询失败都会使本次采样失败，空数据不会掩盖监控断开。
func (client *Client) healthSample(ctx context.Context) error {
	// 顺序查询便于报告具体失败条件；一项失败就结束本次采样，稍后重试。
	for index, expression := range healthyQueries() {
		ok, err := client.Query(ctx, expression)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("健康条件 %d 尚未满足", index+1)
		}
	}
	alerts, err := client.Alerts(ctx)
	if err != nil {
		return err
	}
	if len(alerts) > 0 {
		return fmt.Errorf("仍有告警：%v", alerts)
	}
	return nil
}

// Healthy 每次坏样本都清空连续计时；不能把分散的健康时间累加成一分钟。
func (client *Client) Healthy(ctx context.Context) (result Result, err error) {
	start := time.Now()
	defer func() { result.Elapsed = time.Since(start) }()
	var stableSince time.Time
	for {
		if err := client.healthSample(ctx); err != nil {
			stableSince = time.Time{}
			result.LastProblem = err.Error()
		} else {
			if stableSince.IsZero() {
				stableSince = time.Now()
			}
			if time.Since(stableSince) >= client.stable {
				return result, nil
			}
		}
		if err := wait(ctx, client.poll); err != nil {
			return result, fmt.Errorf("未持续达到健康基线：%s：%w", result.LastProblem, err)
		}
	}
}

// wait 允许 Ctrl+C 及时结束等待，之后 app 仍可用独立预算做恢复。
func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
