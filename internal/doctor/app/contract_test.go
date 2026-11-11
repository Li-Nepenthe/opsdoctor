// 本文件：核对 doctorctl 接受的命令及选项，将校验后的系统、工具和参数与实际解析结果比较。

package app

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"slices"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestDoctorctlDefaultURL 让传输层在拨号边界记录目的地，既能看到缺省主机和端口，也无需启动相应服务。
func TestDoctorctlDefaultURL(t *testing.T) {
	// doctorctl 会复制默认传输层；在复制前设置内存拨号器才能观察真实的目的地址。
	original := http.DefaultTransport
	defer func() { http.DefaultTransport = original }()
	dials := make(chan string, 1)
	http.DefaultTransport = &http.Transport{DialContext: func(_ context.Context, _, addr string) (net.Conn, error) {
		dials <- addr
		return nil, errors.New("测试在拨号前结束")
	}}
	reader := config.NewReader(func(name string) (string, bool) { return "example-token", name == "DOCTOR_TOKEN" })
	if err := RunCLI(t.Context(), []string{"target", "list"}, reader, io.Discard); err == nil {
		t.Fatal("拨号错误没有返回")
	}
	select {
	case addr := <-dials:
		if addr != "127.0.0.1:8090" {
			t.Fatal(addr)
		}
	default:
		t.Fatal("没有观察到医生的默认地址")
	}
}

// TestDoctorctlCLIContract 同时锁住 --args 的省略行为和显式 JSON，避免调整解析顺序后丢了工具参数。
func TestDoctorctlCLIContract(t *testing.T) {
	if _, err := parseCommand(nil); err == nil || err.Error() != "用法：doctorctl target list | tool list --target 系统 | tool run --target 系统 工具 --args JSON" {
		t.Fatalf("公开用法变化：%v", err)
	}
	// 同时核对解析后的值，避免帮助文字仍然正确、选项却已不生效。
	for _, test := range []struct {
		args []string
		want cliCommand
	}{
		{[]string{"target", "list"}, cliCommand{kind: "targets", args: "{}"}},
		{[]string{"tool", "list", "--target", "shop"}, cliCommand{kind: "tools", target: "shop", args: "{}"}},
		{[]string{"tool", "run", "--target", "shop", "mysql_status"}, cliCommand{kind: "run", target: "shop", tool: "mysql_status", args: "{}"}},
		{[]string{"tool", "run", "mysql_status", "--args", `{"limit":3}`, "--target", "shop"}, cliCommand{kind: "run", target: "shop", tool: "mysql_status", args: `{"limit":3}`}},
	} {
		got, err := parseCommand(test.args)
		if err != nil || got != test.want {
			t.Fatalf("%v：%+v，%v", test.args, got, err)
		}
	}
}

// TestDoctorctlEnvironmentContract 缺令牌时必须停在 HTTP 请求之前，地址和令牌均从注入的环境读取。
func TestDoctorctlEnvironmentContract(t *testing.T) {
	var keys []string
	reader := config.NewReader(func(name string) (string, bool) {
		keys = append(keys, name)
		return "", false
	})
	err := RunCLI(t.Context(), []string{"target", "list"}, reader, io.Discard)
	if err == nil || err.Error() != "环境变量 DOCTOR_TOKEN 不能为空" || !slices.Equal(keys, []string{"DOCTOR_URL", "DOCTOR_TOKEN"}) {
		t.Fatalf("查找 %v：%v", keys, err)
	}
}
