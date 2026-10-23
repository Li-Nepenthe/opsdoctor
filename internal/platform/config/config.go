// 本文件：把环境变量转换为有范围约束的配置值。
//
// app 在建立资源前调用 Reader，错误配置应在启动时暴露。
// 读取函数可以注入，测试不必改进程环境。
// 先看 String 如何区分未设置和空值，再看 Int、Duration、Bool 的额外规则。

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Reader 持有环境变量的读取方式；测试可以传入内存中的假环境，避免污染其他测试。
// lookup 在构造时确定；测试传内存查找，真实 app 传环境查找，规则本身不用知道数据来自哪里。
type Reader struct {
	// lookup 区分“没有设置”和“设置为空”，后者通常意味着配置错误。
	lookup func(string) (string, bool)
}

// New 把真实环境变量接到读取器，供各 app 在启动时使用。
func New() Reader {
	return NewReader(os.LookupEnv)
}

// NewReader 允许测试替换配置来源；lookup 的 bool 用来区分“没设置”和“设成空串”。
func NewReader(lookup func(string) (string, bool)) Reader {
	return Reader{lookup: lookup}
}

// String 返回去除首尾空白的配置；未设置时采用 fallback，显式空值返回带变量名的错误。
// 同一个读取器也会读取密码，所以错误只指出变量名和问题，不带原始值。
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
// 这样配置文件里只有两种直观写法，读者不必记住标准库还接受哪些缩写。
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
