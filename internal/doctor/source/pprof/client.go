// 本文件：从指定服务的管理端口读取 goroutine 文本，以函数名和文件位置合并调用栈，按出现数量排序。

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
	var stack stackSample
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			stack.flush(groups)
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 1 && fields[1] == "@" {
			stack.flush(groups)
			value, err := strconv.Atoi(fields[0])
			if err != nil || value <= 0 {
				return nil, fmt.Errorf("goroutine 分组数量不合法")
			}
			stack.count = value
			continue
		}
		if strings.HasPrefix(line, "#") && stack.count > 0 && len(fields) >= 3 {
			stack.addFrame(fields)
		}
	}
	if scanner.Err() != nil {
		return nil, fmt.Errorf("goroutine 文本行超过限制")
	}
	stack.flush(groups)
	if len(groups) == 0 {
		return nil, fmt.Errorf("没有读到可识别的 goroutine 堆栈")
	}
	return sortedGroups(groups), nil
}

// stackSample 暂存一份尚未读完的栈，直到下个分组、空行或文件结束才会合计数量。
type stackSample struct {
	count  int
	frames []string
}

// addFrame 保留函数名和文件位置。内存地址和函数内偏移会随构建变化，不能拿来分组。
func (stack *stackSample) addFrame(fields []string) {
	function, _, _ := strings.Cut(fields[2], "+0x")
	frame := function
	if len(fields) > 3 {
		frame += " " + strings.Join(fields[3:], " ")
	}
	stack.frames = append(stack.frames, frame)
}

func (stack *stackSample) flush(groups map[string]int) {
	if stack.count > 0 && len(stack.frames) > 0 {
		groups[strings.Join(stack.frames, "\n")] += stack.count
	}
	*stack = stackSample{}
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
