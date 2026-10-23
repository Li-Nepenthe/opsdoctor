// 本文件：验证拦截器顺序与日志脱敏。
//
// TestInterceptors 直接组合两个拦截器，用成功、普通错误和 panic 三条路径检查结果。
// 这里不验证网络编解码；先观察内层返回的状态如何被外层记录，再看敏感字符串不得出现的断言。

package grpcx

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestInterceptors 验证日志在恢复层外面，因此成功、普通错误、panic 都会记录最终状态且不泄密。
func TestInterceptors(t *testing.T) {
	cases := []struct {
		name string     // name 描述 handler 的退出方式。
		code codes.Code // code 是日志和返回值共同应有的状态。
	}{
		{"success", codes.OK},        // 正常响应必须保留返回值。
		{"error", codes.Unavailable}, // 已知错误码不能被恢复层改变。
		{"panic", codes.Internal},    // panic 要恢复，但不能写出 panic 值。
	}
	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := logging.New(&output, "test", slog.LevelInfo)
			info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Call"}
			handler := func(ctx context.Context, _ any) (any, error) {
				if logging.FromContext(ctx).RequestID == "" {
					t.Error("没有 request_id")
				}
				switch testcase.name {
				case "panic":
					panic("secret-body")
				case "error":
					return nil, status.Error(codes.Unavailable, "secret-body")
				default:
					return "ok", nil
				}
			}
			inner := func(ctx context.Context, request any) (any, error) {
				return Recovery(logger)(ctx, request, info, handler)
			}
			response, err := Logging(logger)(t.Context(), "secret-body", info, inner)
			if status.Code(err) != testcase.code {
				t.Fatalf("错误码：%v", err)
			}
			if testcase.code == codes.OK && response != "ok" {
				t.Fatalf("返回值：%v", response)
			}
			if strings.Contains(output.String(), "secret-body") || !strings.Contains(output.String(), `"code":"`+testcase.code.String()+`"`) {
				t.Fatalf("日志泄漏或状态错误：%s", output.String())
			}
		})
	}
}
