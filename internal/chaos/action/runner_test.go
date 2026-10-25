// 本文件：用假的命令执行器与本地 HTTP 服务检查注入和恢复操作。
//
// 不连接 Docker 或真实数据库。测试关心发给外部系统的动作范围，以及恢复是否使用原配置。
//
// 主要内容：
//   - TestDeploymentRestore：恢复原镜像内容 ID 和配置，不重新构建。
//   - TestToxicLifecycle：检查延迟规则的请求与重复恢复。
//   - TestInvalidActionNeverRuns：非法动作不能到达外部执行器。
//
// 阅读建议：恢复测试中的旧镜像与当前标签故意不同，用来发现误用标签的问题。

package action

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
)

// TestDeploymentRestore 捕获命令参数，不真的重建容器。
func TestDeploymentRestore(t *testing.T) {
	var args []string
	var environment map[string]string
	runner := &Runner{RunCommand: func(_ context.Context, actual []string, values map[string]string) ([]byte, error) {
		args = actual
		environment = values
		return nil, nil
	}}
	baseline := Baseline{Services: map[string]Deployment{"inventory-service": {Image: "sha256:original", Environment: map[string]string{"DB_MAX_OPEN_CONNS": "20", "DB_MAX_IDLE_CONNS": "10"}}}}
	step := scenario.Step{Action: "deploy", Service: "inventory-service", Environment: map[string]string{"DB_MAX_OPEN_CONNS": "2"}}
	if err := runner.Restore(t.Context(), step, baseline); err != nil {
		t.Fatal(err)
	}
	// 原镜像和原参数要一起恢复，只换其中一个会留下半份错误配置。
	if !slices.Contains(args, "--no-build") || slices.Contains(args, "--build") || environment["INVENTORY_IMAGE"] != "sha256:original" || environment["DB_MAX_OPEN_CONNS"] != "20" {
		t.Fatalf("args=%v env=%v", args, environment)
	}
}

// TestToxicLifecycle 真实走 JSON 编解码，检查规则作用方向和删除路径。
func TestToxicLifecycle(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.Method)
		if request.Method == http.MethodPost {
			var body struct {
				Name, Type, Stream string
				Attributes         struct{ Latency int }
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Name != "transport-delay" || body.Type != "latency" || body.Stream != "downstream" || body.Attributes.Latency != 80 {
				t.Errorf("错误规则：%+v", body)
			}
			writer.WriteHeader(http.StatusOK)
		} else {
			if request.URL.Path != "/proxies/order-inventory/toxics/transport-delay" {
				t.Errorf("错误恢复路径 %s", request.URL.Path)
			}
			// 规则已经不存在也算恢复成功，使中断后的恢复可重复执行。
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	runner := &Runner{HTTP: server.Client(), ProxyURL: server.URL}
	if err := runner.Execute(t.Context(), scenario.Step{Action: "latency", Proxy: "order-inventory", Stream: "downstream", LatencyMS: 80}); err != nil {
		t.Fatal(err)
	}
	if err := runner.Execute(t.Context(), scenario.Step{Action: "clear_latency", Proxy: "order-inventory"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(methods, []string{"POST", "DELETE"}) {
		t.Fatal(methods)
	}
}

// TestInvalidActionNeverRuns 验证运行器本身也做校验，而不只相信 YAML 加载器。
func TestInvalidActionNeverRuns(t *testing.T) {
	runner := &Runner{RunCommand: func(context.Context, []string, map[string]string) ([]byte, error) {
		t.Fatal("非法动作到达 docker")
		return nil, nil
	}}
	if err := runner.Execute(t.Context(), scenario.Step{Action: "stop", Service: "shop-mysql"}); err == nil {
		t.Fatal("非法动作未拒绝")
	}
}
