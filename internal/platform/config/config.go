// 本文件：服务启动时用 Reader 检查环境变量，配置有误便可在连接外部服务前退出。
// 从 String 读起，整数、时长和布尔值都沿用它选择默认值的规则。

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Reader 从配置来源读取并校验环境变量。值可能含有密码，错误消息只给变量名和填写要求。
type Reader struct {
	lookup func(string) (string, bool)
}

// New 从当前进程的环境读取配置。
func New() Reader {
	return NewReader(os.LookupEnv)
}

// NewReader 使用调用方提供的查找函数。只有变量不存在时，查找函数才应返回 false。
func NewReader(lookup func(string) (string, bool)) Reader {
	return Reader{lookup: lookup}
}

// String 去掉首尾空白。变量未设置时采用默认值，显式填入空白则报错，以便发现部署时漏填的配置。
func (reader Reader) String(name, fallback string) (string, error) {
	value, exists := reader.lookup(name)
	if !exists {
		value = fallback
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("环境变量 %s 不能为空", name)
	}
	return value, nil
}

// Int 读取十进制整数，输入和默认值都必须落在给定的闭区间内。
func (reader Reader) Int(name string, fallback, minimum, maximum int) (int, error) {
	text, err := reader.String(name, strconv.Itoa(fallback))
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(text)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("环境变量 %s 必须是 %d 到 %d 之间的整数", name, minimum, maximum)
	}
	return value, nil
}

// Duration 读取指定范围内的时长。非零值必须写明单位，以免误读超时配置。
func (reader Reader) Duration(name string, fallback, minimum, maximum time.Duration) (time.Duration, error) {
	text, err := reader.String(name, fallback.String())
	if err != nil {
		return 0, err
	}
	value, err := time.ParseDuration(text)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("环境变量 %s 必须是 %s 到 %s 之间的时长（例如 5s）", name, minimum, maximum)
	}
	return value, nil
}

// Bool 只接受小写的 true 和 false，让部署文件中的开关含义明确。
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
		return false, fmt.Errorf("环境变量 %s 必须是 true 或 false", name)
	}
}
