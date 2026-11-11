// Package judge 让独立评委核对医生的机制判断和证据引用。只有评委能看到标准答案，评委没有业务工具，也不能改写医生的步骤。
// 先读 rubric.md，再跟随 Evaluate 到 Check，查看调用和校验怎样共用同一份待评清单。
//
//   - grade.go：构造评委请求，调用绑定到账本的客户端并校验回复。
//   - protocol.go：列出待评编号并构造约束这些编号的评分 Schema。
//   - mock.go：离线返回标有 Mock 的固定评分，验证协议能走通。
package judge
