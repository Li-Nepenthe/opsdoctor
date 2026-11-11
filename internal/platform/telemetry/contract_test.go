// 本文件：在不启用导出时核对环境中的服务名和端点校验。

package telemetry

import (
	"testing"
)

// TestTracingEnvironmentContract 三个业务进程使用同一入口；未配置服务名时沿用调用方的命令名。
func TestTracingEnvironmentContract(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	for _, test := range []struct{ name, want string }{{"", "inventory-service"}, {" renamed ", "renamed"}} {
		t.Setenv("OTEL_SERVICE_NAME", test.name)
		runtime, err := New(t.Context(), "inventory-service")
		if err != nil {
			t.Fatal(err)
		}
		if err := runtime.Shutdown(t.Context()); err != nil {
			t.Fatal(err)
		}
		if runtime.Service != test.want {
			t.Fatalf("服务名 %q，期望 %q", runtime.Service, test.want)
		}
	}
	// 拒绝非法协议要发生在创建实际 OTLP 客户端之前。
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "file:///invalid")
	if _, err := New(t.Context(), "order-service"); err == nil || err.Error() != "OTEL_EXPORTER_OTLP_ENDPOINT 必须是不含身份信息的 HTTP 或 HTTPS 地址" {
		t.Fatal(err)
	}
}
