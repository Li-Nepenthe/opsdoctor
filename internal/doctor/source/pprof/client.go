// 本文件：读取配置中某个服务的 goroutine 文本，并按堆栈合并数量。
//
// 大量任务可能停在同一处。只返回最常见的几组，比重复打印几万个相同堆栈更适合诊断。
// 管理端口从配置映射里选择，参数里不能提供 URL。
//
// 主要内容：
//   - New、Close：建立和关闭每个服务的固定客户端。
//   - Goroutines：只访问 goroutine?debug=1，不开放其他 profile 或管理路径。
//   - parse：去掉地址，将相同函数栈合并后按数量排序。
//
// 阅读建议：先看 Goroutines 的服务查找，再看 parse 怎样识别分组首行。

package pprof

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source/httpbackend"
)

// Client 复制配置映射，调用方之后修改原 map 也不能改变已绑定的端点。
type Client struct {
	services map[string]*httpbackend.Client
}

// New 遇到任一地址错误就关闭已创建的客户端，不留下半套配置。
func New(addresses map[string]string) (*Client, error) {
	client := &Client{services: make(map[string]*httpbackend.Client)}
	for service, address := range addresses {
		backend, err := httpbackend.New(address)
		if err != nil {
			client.Close()
			return nil, fmt.Errorf("pprof 配置地址不合法")
		}
		client.services[service] = backend
	}
	return client, nil
}

// Close 释放全部服务的连接；空映射也能正常关闭。
func (client *Client) Close() {
	for _, backend := range client.services {
		backend.Close()
	}
}

// Goroutines 必须先找到服务，不能把未知名称当成主机名尝试连接。
func (client *Client) Goroutines(ctx context.Context, service string) ([]source.StackGroup, error) {
	backend, ok := client.services[service]
	if !ok {
		return nil, fmt.Errorf("服务没有配置 pprof")
	}
	data, err := backend.Bytes(ctx, "/debug/pprof/goroutine", url.Values{"debug": {"1"}})
	if err != nil {
		return nil, err
	}
	return parse(data)
}

// parse 使用 debug=1 已带的组内数量，再把仅地址不同的函数栈合并。
func parse(data []byte) ([]source.StackGroup, error) {
	groups := make(map[string]int)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	count := 0
	var stack []string
	flush := func() {
		if count > 0 && len(stack) > 0 {
			groups[strings.Join(stack, "\n")] += count
		}
		count = 0
		stack = nil
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			flush()
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 1 && fields[1] == "@" {
			flush()
			value, err := strconv.Atoi(fields[0])
			if err != nil || value <= 0 {
				return nil, fmt.Errorf("goroutine 分组数量不合法")
			}
			count = value
			continue
		}
		if strings.HasPrefix(line, "#") && count > 0 && len(fields) >= 3 {
			// 通常是 #、地址、函数+偏移、文件位置。地址会随重启变化，不能作为分组依据。
			function, _, _ := strings.Cut(fields[2], "+0x")
			frame := function
			if len(fields) > 3 {
				frame += " " + strings.Join(fields[3:], " ")
			}
			stack = append(stack, frame)
		}
	}
	if scanner.Err() != nil {
		return nil, fmt.Errorf("goroutine 文本行超过限制")
	}
	flush()
	if len(groups) == 0 {
		return nil, fmt.Errorf("没有读到可识别的 goroutine 堆栈")
	}
	return sortedGroups(groups), nil
}

// sortedGroups 把最常见的堆栈排在前面，同数量时按文本排序，方便比较两次采样。
func sortedGroups(groups map[string]int) []source.StackGroup {
	result := make([]source.StackGroup, 0, len(groups))
	for stack, count := range groups {
		result = append(result, source.StackGroup{Count: count, Stack: stack})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Stack < result[j].Stack
		}
		return result[i].Count > result[j].Count
	})
	return result
}
