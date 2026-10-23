// Package httpx 集中处理 HTTP 入口的通用保护与统一响应。
//
// HTTP app 安装中间件，handler 调用绑定与响应函数；本包依赖 Gin 和 logging，不实现业务接口。
//
// 包内文件：
//   - doc.go：说明包的位置、边界和阅读路线。
//   - httpx.go：HTTP 请求进入业务前后的通用保护。
//   - httpx_test.go：验证 HTTP 中间件组合后的实际响应。
//
// 阅读顺序：httpx.go → httpx_test.go。
package httpx
