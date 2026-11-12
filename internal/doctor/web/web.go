// 本文件：通过固定路径提供编译进医生的三个页面资源。Handler 只开放这些文件，新增资源不会自动成为公开接口。
// 工具和模型返回的文字交给浏览器用 textContent 显示。

package web

import (
	"embed"
	"net/http"
)

// assets 是编译时固定的页面资源，不保存登录状态或用户输入。
//
//go:embed static/index.html static/app.js static/style.css
var assets embed.FS

// Handler 只允许 GET 读取白名单文件，未知路径不回退成首页掩盖接口拼写错误。
func Handler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		file, kind := "", ""
		// 白名单固定 MIME 类型，用户输入不会成为文件系统路径。
		switch request.URL.Path {
		case "/":
			file, kind = "index.html", "text/html; charset=utf-8"
		case "/assets/app.js":
			file, kind = "app.js", "application/javascript; charset=utf-8"
		case "/assets/style.css":
			file, kind = "style.css", "text/css; charset=utf-8"
		default:
			http.NotFound(writer, request)
			return
		}
		body, err := assets.ReadFile("static/" + file)
		if err != nil {
			http.Error(writer, "页面资源不可用", 500)
			return
		}
		writer.Header().Set("Content-Type", kind)
		_, _ = writer.Write(body)
	})
}
