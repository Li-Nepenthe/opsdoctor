// Package web 将医生页面与服务端编译在一起。页面展示诊断、审批预览，也允许用户提交决定或取消诊断；身份、参数和预算由服务端校验。
// 先读 Handler 的固定路径，再看 static/app.js 如何把告警、模型回复和工具结果当作文字显示。
//
//   - web.go：嵌入 HTML、JavaScript 和 CSS，并限制可读取的文件和请求方法。
package web
