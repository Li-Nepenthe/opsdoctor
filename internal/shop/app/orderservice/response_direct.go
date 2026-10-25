// 本文件：定义当前版本的响应收尾方式。
//
// 路由装配统一调用 responseRecorder，构建时只选择一份实现。
// 当前实现保留既有请求路径，响应日志仍由原来的中间件记录。
//
// 主要内容：
//   - responseRecorder：让后续处理器完成请求。
//
// 阅读建议：先看 app.go 中中间件的登记位置，再看这一层如何交出处理权。

//go:build !leakbug

package orderservice

import "github.com/gin-gonic/gin"

// responseRecorder 把请求交给后续处理器，响应内容和状态由业务决定。
func responseRecorder() gin.HandlerFunc {
	return func(ctx *gin.Context) { ctx.Next() }
}
