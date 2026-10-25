// 本文件：用假的命令执行器与本地 HTTP 服务检查注入和恢复操作。
//
// 不连接 Docker 或真实数据库。测试关心发给外部系统的动作范围，以及恢复是否使用原配置。
//
// 主要内容：
//   - TestDeploymentRestore：恢复原镜像内容 ID 和配置，不重新构建。
//   - TestToxicLifecycle：检查延迟规则的请求与重复恢复。
//   - TestInvalidActionNeverRuns：非法动作不能到达外部执行器。
//   - TestCapturePinsOriginalImage：发布前给原内容留下独立引用。
//   - TestRestoreRejectsChangedImage：标签被外部改动后拒绝恢复错误版本。
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
		if actual[0] == "image" {
			return []byte("sha256:original\n"), nil
		}
		args = actual
		environment = values
		return nil, nil
	}}
	baseline := Baseline{Services: map[string]Deployment{"inventory-service": {Image: "opsdoctor/inventory-service:saved-original", ImageID: "sha256:original", Environment: map[string]string{"DB_MAX_OPEN_CONNS": "20", "DB_MAX_IDLE_CONNS": "10"}}}}
	step := scenario.Step{Action: "deploy", Service: "inventory-service", Environment: map[string]string{"DB_MAX_OPEN_CONNS": "2"}}
	if err := runner.Restore(t.Context(), step, baseline); err != nil {
		t.Fatal(err)
	}
	// 原镜像和原参数要一起恢复，只换其中一个会留下半份错误配置。
	if !slices.Contains(args, "--no-build") || !slices.Contains(args, "never") || slices.Contains(args, "--build") || environment["INVENTORY_IMAGE"] != "opsdoctor/inventory-service:saved-original" || environment["DB_MAX_OPEN_CONNS"] != "20" {
		t.Fatalf("args=%v env=%v", args, environment)
	}
}

// TestCapturePinsOriginalImage 模拟标签稍后会被覆盖，检查快照阶段已保留原内容。
func TestCapturePinsOriginalImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"settings":{"qps":200,"hot_sku_id":0}}}`))
	}))
	defer server.Close()
	var pinned bool
	runner := &Runner{HTTP: server.Client(), LoadURL: server.URL}
	runner.RunCommand = func(_ context.Context, args []string, _ map[string]string) ([]byte, error) {
		switch args[0] {
		case "compose":
			return []byte("container-id"), nil
		case "inspect":
			return []byte(`{"image":"sha256:original","environment":["DB_MAX_OPEN_CONNS=20","MYSQL_DSN=private"]}`), nil
		case "image":
			pinned = slices.Equal(args, []string{"image", "tag", "sha256:original", "opsdoctor/inventory-service:saved-original"})
			return nil, nil
		default:
			t.Fatalf("意外命令：%v", args)
			return nil, nil
		}
	}
	baseline, err := runner.Capture(t.Context(), scenario.Scenario{Inject: []scenario.Step{{Action: "deploy", Service: "inventory-service"}}})
	saved := baseline.Services["inventory-service"]
	// 原内容、标签、公开配置都要有，但凭据不能跟着 Docker 的完整环境列表落盘。
	if err != nil || !pinned || saved.ImageID != "sha256:original" || saved.Image != "opsdoctor/inventory-service:saved-original" || saved.Environment["DB_MAX_OPEN_CONNS"] != "20" || saved.Environment["MYSQL_DSN"] != "" {
		t.Fatalf("没有可靠保存基线：%v pinned=%t", err, pinned)
	}
}

// TestRestoreRejectsChangedImage 不允许标签同名就认定程序相同。
func TestRestoreRejectsChangedImage(t *testing.T) {
	runner := &Runner{RunCommand: func(_ context.Context, args []string, _ map[string]string) ([]byte, error) {
		if args[0] != "image" {
			t.Fatal("内容不匹配时仍执行了部署")
		}
		return []byte("sha256:changed"), nil
	}}
	baseline := Baseline{Services: map[string]Deployment{"inventory-service": {Image: "saved-image", ImageID: "sha256:original"}}}
	if err := runner.Restore(t.Context(), scenario.Step{Action: "deploy", Service: "inventory-service"}, baseline); err == nil {
		t.Fatal("错误镜像被接受")
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
