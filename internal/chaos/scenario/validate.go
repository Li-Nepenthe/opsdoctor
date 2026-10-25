// 本文件：把场景操作限制在计划书规定的服务和资源内。
//
// YAML 不提供任意命令入口。表名、索引名、代理名与部署变量都有固定范围。
// 运行器也会调用这些校验，避免只有文件加载路径受到保护。
//
// 主要内容：
//   - ValidateStep：按动作检查参数。
//   - ValidateEnvironment：拒绝密码变量和未开放的配置。
//
// 阅读建议：先看索引配对，再看部署变量为什么只列白名单。

package scenario

import (
	"fmt"
	"strconv"
	"time"
)

// ValidateStep 先限制动作类型，再检查必要参数。运行器不会拼接未经校验的 SQL 标识符。
func ValidateStep(step Step) error {
	// 每种动作只开放必要资源；合法表名和合法索引名还必须正确配对。
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
		if step.Image != "" && (step.Service != "order-service" || (step.Image != "opsdoctor/order-service:20261024.3" && step.Image != "opsdoctor/order-service:20261024.4")) {
			break
		}
		return ValidateEnvironment(step.Environment)
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
		// 指针为空表示保持原值；因此至少要明确更新一个字段。
		if step.QPS == nil && step.HotSKUID == nil {
			break
		}
		if step.QPS != nil && (*step.QPS < 0 || *step.QPS > 10000) {
			break
		}
		if step.HotSKUID != nil && (*step.HotSKUID < 0 || *step.HotSKUID > 500000) {
			break
		}
		return nil
	case "delete_cache", "expire_cache":
		// 先限制范围，再计算尾编号，既避免越界，也防止大整数相加溢出。
		if step.FirstSKUID < 1 || step.Count < 1 || step.Count > 10000 || step.FirstSKUID > 500001-int64(step.Count) {
			break
		}
		if step.Action == "delete_cache" || (step.TTLSeconds >= 1 && step.TTLSeconds <= 60) {
			return nil
		}
	}
	return fmt.Errorf("动作 %q 的类型或参数不合法", step.Action)
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
