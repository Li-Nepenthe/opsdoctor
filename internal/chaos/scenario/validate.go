// 本文件：场景配置和运行器共用输入校验。
// ValidateStep 限定可操作的资源，ValidateEnvironment 单独检查环境变量，未开放的配置不能进入操作。

package scenario

import (
	"fmt"
	"reflect"
	"strconv"
	"time"
)

// ValidateStep 先限制动作类型，再检查必要参数。运行器不会拼接未经校验的 SQL 标识符。
func ValidateStep(step Step) error {
	// 索引名和表名必须配对，分别合法仍不足以通过检查。
	switch step.Action {
	case "drop_index", "restore_index":
		if (step.Table == "stock" && step.Index == "uk_stock_sku") || (step.Table == "orders" && step.Index == "idx_orders_user_created") {
			return nil
		}
	case "deploy":
		// 只发布两个业务入口，不允许把 Docker 命令变成任意容器管理器。
		if step.Service != "order-service" && step.Service != "inventory-service" {
			break
		}
		if step.Image != "" && (step.Service != "order-service" || (step.Image != "opsdoctor/order-service:20261026.1" && step.Image != "opsdoctor/order-service:20261026.2")) {
			break
		}
		return ValidateEnvironment(step.Environment)
	case "restart", "probe_log":
		// 对照与注入只操作订单入口，不能携带新环境变量、镜像或任意请求地址。
		if step.Service == "order-service" && reflect.DeepEqual(step, Step{Action: step.Action, Service: step.Service}) {
			return nil
		}
	case "stop", "start":
		if step.Service == "order-worker" {
			return nil
		}
	case "latency", "clear_latency":
		if step.Proxy != "order-inventory" && step.Proxy != "inventory-mysql" && step.Proxy != "order-redis" {
			break
		}
		if step.Action == "clear_latency" {
			return nil
		}
		if step.LatencyMS >= 1 && step.LatencyMS <= 2000 && (step.Stream == "upstream" || step.Stream == "downstream") {
			return nil
		}
	case "load":
		if validLoadSettings(step) {
			return nil
		}
	case "burst":
		// 批数与每批请求数分别受限，保持实验在既定流量范围内。
		if validSKURange(step.FirstSKUID, step.Count, 500) && step.Requests >= 1 && step.Requests <= 64 {
			return nil
		}
	case "delete_cache", "expire_cache":
		if validSKURange(step.FirstSKUID, step.Count, 10000) &&
			(step.Action == "delete_cache" || (step.TTLSeconds >= 1 && step.TTLSeconds <= 60)) {
			return nil
		}
	}
	return fmt.Errorf("动作 %q 的类型或参数不合法", step.Action)
}

// 省略的字段保持原值，至少要指定一项变更。
func validLoadSettings(step Step) bool {
	if step.QPS == nil && step.HotSKUID == nil {
		return false
	}
	return (step.QPS == nil || (*step.QPS >= 0 && *step.QPS <= 10000)) &&
		(step.HotSKUID == nil || (*step.HotSKUID >= 0 && *step.HotSKUID <= 500000))
}

// 先限定批次大小，再从末端倒算允许的起点，避免大整数相加时溢出。
func validSKURange(first int64, count, maxCount int) bool {
	return first >= 1 && count >= 1 && count <= maxCount && first <= 500001-int64(count)
}

// ValidateEnvironment 不接受 DSN、口令或任意环境变量，只允许实验所需的公开配置。
func ValidateEnvironment(values map[string]string) error {
	// 未列出的变量一律拒绝，而不是先接受再试图遮盖其中的密码。
	for key, value := range values {
		var err error
		switch key {
		case "CACHE_SINGLEFLIGHT", "INVENTORY_BREAKER_ENABLED":
			if value != "true" && value != "false" {
				err = fmt.Errorf("须为 true 或 false")
			}
		case "DB_MAX_OPEN_CONNS", "DB_MAX_IDLE_CONNS", "INVENTORY_GETSTOCK_MAX_RETRIES", "RATE_LIMIT_RPS", "RATE_LIMIT_BURST":
			err = validateInteger(key, value)
		case "CACHE_TTL_JITTER", "INVENTORY_GETSTOCK_ATTEMPT_TIMEOUT", "INVENTORY_RETRY_BACKOFF", "INVENTORY_RETRY_MIN_BUDGET":
			var duration time.Duration
			duration, err = time.ParseDuration(value)
			maximum := time.Minute
			if key == "INVENTORY_GETSTOCK_ATTEMPT_TIMEOUT" {
				maximum = 10 * time.Second
			}
			if key == "INVENTORY_RETRY_BACKOFF" || key == "INVENTORY_RETRY_MIN_BUDGET" {
				maximum = time.Second
			}
			if duration < 0 || duration > maximum || (key == "INVENTORY_RETRY_MIN_BUDGET" && duration < time.Millisecond) {
				err = fmt.Errorf("时长超出范围")
			}
		default:
			return fmt.Errorf("不允许部署变量 %s", key)
		}
		if err != nil {
			return fmt.Errorf("部署变量 %s 不合法", key)
		}
	}
	return nil
}

// validateInteger 提前使用服务端同样的单项边界，避免明知无效还去重建容器。
func validateInteger(key, value string) error {
	// 默认范围给入口限流使用，连接数和重试次数再收紧，零只在允许关闭或不用时有效。
	minimum, maximum := 0, 100000
	switch key {
	case "DB_MAX_OPEN_CONNS":
		minimum, maximum = 1, 100
	case "DB_MAX_IDLE_CONNS":
		maximum = 100
	case "INVENTORY_GETSTOCK_MAX_RETRIES":
		maximum = 10
	case "RATE_LIMIT_RPS", "RATE_LIMIT_BURST":
		minimum = 1
	}
	number, err := strconv.Atoi(value)
	if err != nil || number < minimum || number > maximum {
		return fmt.Errorf("整数超出范围")
	}
	return nil
}
