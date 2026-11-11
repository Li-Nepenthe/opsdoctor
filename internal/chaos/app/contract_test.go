// 本文件：限定在命令解析和本地配置组装，检查每个入口在执行实验前能识别的参数。

package app

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/action"
	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/verify"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
)

// TestChaosEnvironmentContract 引擎只做装配；观察器遇到参数不匹配会在触碰数据库、运行实验之前退出。
func TestChaosEnvironmentContract(t *testing.T) {
	keys := map[string]bool{}
	load := func(values map[string]string) (*Engine, error) {
		return newEngine(config.NewReader(func(name string) (string, bool) {
			keys[name] = true
			v, ok := values[name]
			return v, ok
		}))
	}
	engine, err := load(nil)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Fields("CHAOS_BASELINE_MYSQL_DSN CHAOS_BASELINE_PRODUCT_COUNT CHAOS_BASELINE_WORKERS CHAOS_MYSQL_DSN CHAOS_ORDER_URL CHAOS_REDIS_ADDR LOADGEN_CONTROL_URL PROMETHEUS_URL REDIS_PASSWORD TOXIPROXY_URL")
	if !slices.Equal(slices.Sorted(maps.Keys(keys)), want) {
		t.Fatalf("环境查找：%v", keys)
	}
	runner := engine.Executor.(*action.Runner)
	observer := engine.Observer.(*verify.Baseline)
	if runner.OrderURL != "http://127.0.0.1:8081" || runner.ProxyURL != "http://127.0.0.1:8474" ||
		runner.LoadURL != "http://127.0.0.1:6070" || runner.RedisAddress != "127.0.0.1:6379" ||
		runner.MySQLDSN != "" || runner.RedisPassword != "" || observer.URL != "http://127.0.0.1:9090" {
		t.Fatal("引擎默认地址或凭据改变")
	}

	// 后端只返回少量本地测试数据；失败原因带有真实注入给观察器的商品数和工作者数。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"code":"OK","data":{"settings":{"product_count":1,"workers":1}}}`)
	}))
	defer server.Close()
	engine, err = load(map[string]string{"LOADGEN_CONTROL_URL": server.URL, "PROMETHEUS_URL": server.URL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if _, err := engine.Observer.Healthy(ctx); err == nil || !strings.Contains(err.Error(), "要求 10000、64") {
		t.Fatalf("压测默认值：%v", err)
	}
	for _, bound := range []struct {
		name     string
		min, max int
	}{{"CHAOS_BASELINE_PRODUCT_COUNT", 1, 500000}, {"CHAOS_BASELINE_WORKERS", 1, 256}} {
		for _, n := range []int{bound.min - 1, bound.min, bound.max, bound.max + 1} {
			_, err := load(map[string]string{bound.name: strconv.Itoa(n)})
			if (err == nil) != (bound.min <= n && n <= bound.max) {
				t.Fatalf("%s=%d：%v", bound.name, n, err)
			}
		}
	}
}

// TestChaosCLIContract 给每个动词一个会在解析阶段退出的参数，不会创建实验记录或执行场景动作。
func TestChaosCLIContract(t *testing.T) {
	reader := config.NewReader(func(string) (string, bool) { return "", false })
	if err := Run(t.Context(), nil, reader, &bytes.Buffer{}); err == nil || err.Error() != "用法：chaos list | inject ID [--qps N --sku ID --latency-ms N] | verify ID | restore ID | restore-all | healthy | selftest" {
		t.Fatalf("公开用法变化：%v", err)
	}
	// 场景目录是相对仓库根解析的；这里只读取仓库中的清单。
	want, err := os.ReadFile("testdata/inject.golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join("..", "..", ".."))
	var out bytes.Buffer
	err = Run(t.Context(), []string{"inject", "S01", "--help"}, reader, &out)
	if !errors.Is(err, flag.ErrHelp) || strings.ReplaceAll(out.String(), "\n    \t", "\n    ") != string(want) {
		t.Fatalf("inject 参数：%v\n%s", err, &out)
	}
	for _, command := range strings.Fields("list selftest healthy restore-all verify restore") {
		args := []string{command}
		if command == "verify" || command == "restore" {
			args = append(args, "S01")
		}
		err := Run(t.Context(), append(args, "--extra"), reader, &out)
		if err == nil || err.Error() != command+" 不接受额外参数" {
			t.Fatalf("%s: %v", command, err)
		}
	}
}
