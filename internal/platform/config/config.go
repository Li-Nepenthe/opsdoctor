package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Reader 持有环境变量的读取方式；测试可以传入内存中的假环境，避免污染其他测试。
// 构造后不修改字段；并发安全性取决于传入的 lookup 是否支持并发读取。
type Reader struct {
	// lookup 区分“没有设置”和“设置为空”，后者通常意味着配置错误。
	lookup func(string) (string, bool)
}

// New 使用操作系统环境变量构造读取器；它不缓存值，不阻塞，不会返回错误。
func New() Reader {
	return NewReader(os.LookupEnv)
}

// NewReader 注入读取函数；lookup 必须非 nil，返回值遵循 os.LookupEnv 的约定。
// 这里只组装依赖，不读环境、不阻塞；调用方应保证 lookup 在使用期间不被替换。
func NewReader(lookup func(string) (string, bool)) Reader {
	return Reader{lookup: lookup}
}

// String 返回去除首尾空白的配置；未设置时采用 fallback，显式空值返回带变量名的错误。
// 错误从不包含原始值，因为同一个读取器也可能读取密码；是否阻塞取决于 lookup。
func (reader Reader) String(name, fallback string) (string, error) {
	value, exists := reader.lookup(name)
	if !exists {
		// 只有“没设置”才采用默认值；显式设置为空通常是部署写错了，应在下面报错。
		value = fallback
	}
	value = strings.TrimSpace(value)
	if value == "" {
		// 默认值为空也说明缺少必需配置；只报变量名，避免把密码等内容带进日志。
		return "", fmt.Errorf("环境变量 %s 不能为空", name)
	}
	return value, nil
}

// Int 读取闭区间 [minimum, maximum] 中的十进制整数；无效格式或越界都会返回错误。
// 默认值也经过同样的校验，防止程序自己提供不合法的默认配置；不会在错误中回显输入。
func (reader Reader) Int(name string, fallback, minimum, maximum int) (int, error) {
	text, err := reader.String(name, strconv.Itoa(fallback))
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(text)
	if err != nil || value < minimum || value > maximum {
		// 格式正确还不够，连接数等配置也有范围限制；统一报允许范围，不回显原始值。
		return 0, fmt.Errorf("环境变量 %s 必须是 %d 到 %d 之间的整数", name, minimum, maximum)
	}
	return value, nil
}

// Duration 读取带单位的时长（例如 800ms、5m），并验证闭区间；单位统一为 time.Duration。
// 不接受无单位数字，以免把秒误当纳秒；调用方传入自己的默认值和边界。
func (reader Reader) Duration(name string, fallback, minimum, maximum time.Duration) (time.Duration, error) {
	text, err := reader.String(name, fallback.String())
	if err != nil {
		return 0, err
	}
	value, err := time.ParseDuration(text)
	if err != nil || value < minimum || value > maximum {
		// 单位解析和业务范围都通过才接受；例如 -1s 有合法语法，但通常不是合法超时。
		return 0, fmt.Errorf("环境变量 %s 必须是 %s 到 %s 之间的时长（例如 5s）", name, minimum, maximum)
	}
	return value, nil
}

// Bool 只接受 true 或 false，避免 1、t 等缩写让配置难以阅读；未设置时使用 fallback。
// 它不修改 Reader；非法值返回带变量名的错误，不包含输入内容。
func (reader Reader) Bool(name string, fallback bool) (bool, error) {
	value, err := reader.String(name, strconv.FormatBool(fallback))
	if err != nil {
		return false, err
	}
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		// 不接受 1、t 等其他常见写法，让环境文件里的真假含义一眼就能看懂。
		return false, fmt.Errorf("环境变量 %s 必须是 true 或 false", name)
	}
}
