// Package agent 把“问模型、查证据、再问模型”组织成一个有边界的循环。
//
// 模型决定下一步想看什么，程序负责检查参数、执行只读工具和保存步骤。工具失败
// 也是一份观察结果，交回模型后它可以修正参数。只有通过校验的 submit_report 才结束诊断。
// 循环有轮数、工具数、token 和时间预算，防止一直查下去。
//
// 文件清单：
//   - loop_test.go：用脚本检查纠错、并行、取消、预算和报告边界。
//   - doc.go：本包解决的问题与阅读顺序。
//   - model.go：步骤、依赖和一次诊断的输入输出。
//   - prompt.go：读取带版本号的嵌入提示词，组装固定前缀。
//   - report.go：报告的 JSON Schema 与证据步骤检查。
//   - schema.go：校验报告使用的有限 JSON Schema 类型。
//   - loop.go：调用模型、记录用量、判断预算和报告。
//   - calls.go：并行执行只读工具，保留调用 ID 与步骤号的对应关系。
//
// 阅读顺序：model.go → prompt.go → loop.go → calls.go → report.go。
// 本包不导入数据库、HTTP、目标系统实现或实验场景，也不执行任何写操作。
package agent
