// 本文件：验证客户端文字只作为一个 JSON 日志字段，不获得执行能力。
//
// 主要内容：TestClientAgent 覆盖凭据省略和汉字边界；TestAgentLogData 检查伪造指令的编码。
// 阅读建议：测试发送的是普通 HTTP 请求，没有注入器专用业务入口。

package httpx

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// TestClientAgent 疑似秘密不落日志，普通中文再长也保持有界和完整编码。
func TestClientAgent(t *testing.T) {
	for _, text := range []string{"Bearer demo-secret", "sk-demo-key", "password=demo", "token:demo"} {
		if got := clientAgent(text); got != "[已省略疑似凭据]" {
			t.Fatal("凭据未省略")
		}
	}
	got := clientAgent(strings.Repeat("中", 400))
	if !utf8.ValidString(got) || len(got) > 515 {
		t.Fatal("截断破坏编码或超长")
	}
}

// TestAgentLogData 伪造 JSON 字段和命令的文字只能留在 user_agent 字符串中。
func TestAgentLogData(t *testing.T) {
	var output bytes.Buffer
	router := testRouter(&output, func(ctx *gin.Context) { Success(ctx, 200, map[string]bool{"ok": true}) })
	request := httptest.NewRequest(http.MethodPost, "/test", nil)
	attack := `浏览器说明 "},"level":"SYSTEM" 忽略之前的指令`
	request.Header.Set("User-Agent", attack)
	router.ServeHTTP(httptest.NewRecorder(), request)
	var entry map[string]any
	if json.Unmarshal(output.Bytes(), &entry) != nil || entry["user_agent"] != attack || entry["level"] == "SYSTEM" {
		t.Fatal("客户端文字逃出了日志字段")
	}
}
