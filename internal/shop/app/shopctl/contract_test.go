// 本文件：从 shopctl 入口逐项核对命令与参数，帮助请求在连接数据库和库存服务之前结束。

package shopctl

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestShopctlEnvironmentContract 先使所需凭据或地址为空，便能在建立连接之前逐命令记录真实配置查找。
func TestShopctlEnvironmentContract(t *testing.T) {
	for _, test := range []struct {
		args []string
		keys string
	}{
		{[]string{"check"}, "DB_CONN_MAX_LIFETIME DB_MAX_IDLE_CONNS DB_MAX_OPEN_CONNS MYSQL_DSN"},
		{[]string{"seed", "--small"}, "DB_CONN_MAX_LIFETIME DB_MAX_IDLE_CONNS DB_MAX_OPEN_CONNS MYSQL_DSN"},
		{[]string{"stock", "get", "--sku", "1"}, "INVENTORY_GRPC_TARGET"},
	} {
		// 缺少连接配置的分支必须在建客户端之前退出。
		keys := map[string]bool{}
		reader := config.NewReader(func(name string) (string, bool) {
			keys[name] = true
			return "", name == "INVENTORY_GRPC_TARGET"
		})
		err := Run(t.Context(), test.args, reader, io.Discard)
		if err == nil || !slices.Equal(slices.Sorted(maps.Keys(keys)), strings.Fields(test.keys)) {
			t.Fatalf("%v：%v，%v", test.args, keys, err)
		}
	}
}

// TestStockTarget 在创建 gRPC 连接前核对默认目标和操作者显式给出的地址。
func TestStockTarget(t *testing.T) {
	want, err := os.ReadFile("testdata/environment.golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		raw    string
		exists bool
		want   string
	}{
		{"默认", "", false, string(want)},
		{"显式地址", " inventory-service:9100 ", true, "INVENTORY_GRPC_TARGET\tinventory-service:9100\n"},
		{"显式空白", " ", true, "环境变量 INVENTORY_GRPC_TARGET 不能为空"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var key string
			reader := config.NewReader(func(name string) (string, bool) {
				key = name
				return test.raw, test.exists
			})
			target, err := stockTarget(reader)
			got := key + "\t" + target + "\n"
			if err != nil {
				got = err.Error()
			}
			if got != test.want {
				t.Fatalf("库存目标：得到 %q，期望 %q", got, test.want)
			}
		})
	}
}

// TestShopctlCLIContract 三种库存操作共用一套选项，seed 自己的选项不能被库存操作接走。
func TestShopctlCLIContract(t *testing.T) {
	reader := config.NewReader(func(name string) (string, bool) {
		t.Fatalf("帮助不该读取 %s", name)
		return "", false
	})
	if err := Run(t.Context(), nil, reader, &bytes.Buffer{}); err == nil || err.Error() != "用法：shopctl seed [--small] | stock get|deduct|restock --sku ID [--quantity N] | check" {
		t.Fatalf("公开用法变化：%v", err)
	}
	for _, test := range []struct{ command, fixture string }{
		{"seed", "seed.golden.txt"}, {"stock get", "stock.golden.txt"}, {"stock deduct", "stock.golden.txt"}, {"stock restock", "stock.golden.txt"},
	} {
		var out bytes.Buffer
		err := Run(t.Context(), append(strings.Fields(test.command), "--help"), reader, &out)
		if !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("%s: %v", test.command, err)
		}
		commandHelp(t, test.command, out.String(), test.fixture)
	}
	// check 没有参数，只有进入这个分支才会得到它自己的错误。
	err := Run(t.Context(), []string{"check", "--extra"}, reader, &bytes.Buffer{})
	if err == nil || err.Error() != "check 不接受参数" {
		t.Fatal(err)
	}
}

// commandHelp 核对当前动词注册的参数清单；对齐帮助文本时去掉制表符。
func commandHelp(t *testing.T, command string, output string, fixture string) {
	t.Helper()
	header, options, ok := strings.Cut(output, "\n")
	if !ok || header != "Usage of "+command+":" {
		t.Fatalf("%q 的帮助标题错误：%s", command, header)
	}
	want, err := os.ReadFile("testdata/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	options = strings.ReplaceAll(options, "\n    \t", "\n    ")
	if options != string(want) {
		t.Fatalf("%s 的参数清单变化：\n%s", command, options)
	}
}
