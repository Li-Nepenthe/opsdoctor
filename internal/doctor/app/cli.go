// 本文件：实现 doctorctl 的三个命令，通过 HTTP 使用正在运行的医生。
//
// CLI 不创建数据源连接，也不自己实现工具校验。这样多开几个终端，仍然使用医生
// 那一个三连接 MySQL 池，不会每个命令都再开三条连接。
//
// 主要内容：
//   - RunCLI：解析命令，读取医生地址与登录令牌，输出结果。
//   - parseCommand：只接受 target list、tool list、tool run 的固定参数。
//   - request：带身份调用同一 HTTP 接口，拒绝重定向并限制响应大小。
//   - printList：从系统列表中选出用户要求查看的工具说明。
//
// 阅读建议：工具 JSON 原样交给服务器，只有那里决定是否合法、超时多久以及怎样截断。

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// cliCommand 不允许 URL、主机、令牌等命令参数；这些是操作者的进程环境配置。
type cliCommand struct {
	kind   string // targets、tools 或 run。
	target string // 人选择的系统，只用于 HTTP 路径，不混入工具参数。
	tool   string // 当前系统注册过的工具名。
	args   string // 原始 JSON，真正校验仍由服务器注册表负责。
}

// RunCLI 的输出可以重定向保存；凭据只放 Authorization 头，不打印请求转储。
func RunCLI(ctx context.Context, args []string, reader config.Reader, output io.Writer) error {
	command, err := parseCommand(args)
	if err != nil {
		return err
	}
	address, err := reader.String("DOCTOR_URL", "http://127.0.0.1:8090")
	if err != nil {
		return err
	}
	token, err := reader.String("DOCTOR_TOKEN", "")
	if err != nil {
		return err
	}
	path := "/api/v1/targets"
	method := http.MethodGet
	if command.kind == "run" {
		path += "/" + command.target + "/tools/" + command.tool + "/run"
		method = http.MethodPost
	}
	data, err := request(ctx, address, token, method, path, command.args)
	if err != nil {
		return err
	}
	if command.kind != "run" {
		return printList(output, command, data)
	}
	var result tools.Result
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("医生返回了无法识别的工具结果")
	}
	// 失败结果同样打印，方便学习时查看原因；非零退出码供脚本判断这一调用没有成功。
	if _, err := fmt.Fprintln(output, string(data)); err != nil {
		return err
	}
	if result.Status != "ok" {
		return fmt.Errorf("工具执行未成功：%s", result.Status)
	}
	return nil
}

// parseCommand 自己读少量固定选项，允许 --args 放在工具名之后，符合文档示例。
func parseCommand(args []string) (cliCommand, error) {
	value := cliCommand{args: "{}"}
	if len(args) < 2 {
		return value, fmt.Errorf("用法：doctorctl target list | tool list --target 系统 | tool run --target 系统 工具 --args JSON")
	}
	switch args[0] + " " + args[1] {
	case "target list":
		if len(args) != 2 {
			return value, fmt.Errorf("target list 不接受选项")
		}
		value.kind = "targets"
		return value, nil
	case "tool list":
		value.kind = "tools"
	case "tool run":
		value.kind = "run"
	default:
		return value, fmt.Errorf("不支持这个命令")
	}
	seen := map[string]bool{}
	for index := 2; index < len(args); index++ {
		item := args[index]
		if item == "--target" || item == "--args" {
			if seen[item] || index+1 >= len(args) || (item == "--args" && value.kind != "run") {
				return value, fmt.Errorf("选项缺值、重复或不适用于本命令")
			}
			seen[item] = true
			index++
			if item == "--target" {
				value.target = args[index]
			} else {
				value.args = args[index]
			}
			continue
		}
		// 未知 flag 必须报错，不能被误当成工具名后发送给服务器。
		if strings.HasPrefix(item, "-") || value.kind != "run" || value.tool != "" {
			return value, fmt.Errorf("不支持的参数")
		}
		value.tool = item
	}
	if !regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`).MatchString(value.target) {
		return value, fmt.Errorf("必须提供合法的 --target 系统名")
	}
	if value.kind == "run" && !regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`).MatchString(value.tool) {
		return value, fmt.Errorf("必须提供合法的工具名")
	}
	return value, nil
}

// request 固定相对路径，不允许医生接口通过重定向把登录令牌带到另一个服务。
func request(ctx context.Context, address, token, method, path, body string) (json.RawMessage, error) {
	base, err := url.Parse(address)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || (base.Path != "" && base.Path != "/") || base.RawQuery != "" || base.Fragment != "" {
		return nil, fmt.Errorf("DOCTOR_URL 必须是不含凭据和路径的 HTTP 地址")
	}
	base.Path = path
	var input io.Reader
	if method == http.MethodPost {
		input = bytes.NewBufferString(body)
	}
	call, err := http.NewRequestWithContext(ctx, method, base.String(), input)
	if err != nil {
		return nil, fmt.Errorf("无法创建医生请求")
	}
	call.Header.Set("Authorization", "Bearer "+token)
	call.Header.Set("Content-Type", "application/json")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(call)
	if err != nil {
		return nil, fmt.Errorf("无法连接医生或请求超时")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("医生接口返回 HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return nil, fmt.Errorf("医生响应不可读或超过上限")
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	} // 不回显服务端的任意错误正文。
	if json.Unmarshal(data, &envelope) != nil || len(envelope.Data) == 0 {
		return nil, fmt.Errorf("医生响应缺少 data")
	}
	return envelope.Data, nil
}

// printList 从服务端给出的菜单选择，不根据本机接入文件猜测可用工具。
func printList(output io.Writer, command cliCommand, data json.RawMessage) error {
	var targets []struct {
		Name  string             `json:"name"`
		Tools []tools.Definition `json:"tools"`
	} // 不接受或展示后端凭据字段。
	if json.Unmarshal(data, &targets) != nil {
		return fmt.Errorf("无法识别医生系统列表")
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if command.kind == "targets" {
		names := []string{}
		for _, target := range targets {
			names = append(names, target.Name)
		}
		return encoder.Encode(names)
	}
	for _, target := range targets {
		if target.Name == command.target {
			return encoder.Encode(target.Tools)
		}
	}
	return fmt.Errorf("系统不存在")
}
