// 本文件：将响应结果交给后台任务整理。
//
// responseRecorder 在请求完成后取得最终状态，不在业务处理途中读取它。
// 后台结果通过 channel 交接，channel 是 Go 中用来传递值的通道。
//
// 主要内容：
//   - responseRecorder：保存响应状态并启动结果整理。
//   - deliverResponse：把状态送到结果通道。
//
// 阅读建议：留意谁负责发送、谁负责接收，以及请求结束后两方是否仍存在。

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

// deliverResponse 使用无缓冲通道交接结果；发送方要等接收方准备好才能结束。
func deliverResponse(result chan<- int, status int) {
	result <- status
}
