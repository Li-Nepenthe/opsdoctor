// 本文件：将响应结果交给后台任务整理。
//
// responseRecorder 在请求完成后取得最终状态，不在业务处理途中读取它。

//go:build leakbug

package orderservice

import "github.com/gin-gonic/gin"

// responseRecorder 等业务处理完成后再取状态，避免把默认的 200 当成最终结果。
func responseRecorder() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ctx.Next()
		result := make(chan int)
		go deliverResponse(result, ctx.Writer.Status())
	}
}

// deliverResponse 在接收方取走最终响应状态时完成交接。
func deliverResponse(result chan<- int, status int) {
	result <- status
}
