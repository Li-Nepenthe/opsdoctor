// Package main 是 archcheck 工具：自动检查代码有没有放错位置、有没有越过模块之间的边界。
//
// 项目对“代码放在哪里、谁能依赖谁”有一些规定（计划书第 5.1 节）。这些规定靠人记容易忘，
// 本工具在 CI 里自动检查。它检查四类问题：
//   - 位置：根目录不能放 .go 文件；cmd/<程序>/ 下只能有一个 main.go；包名不能太含糊。
//   - 依赖方向：医生（doctor）和靶场（shop）不能互相导入代码；
//     通用基础设施（platform）不能导入任何业务代码。
//   - 包内分层：靶场的业务包里，不同角色的文件只能用各自该用的技术，
//     例如 service.go 不能直接导入数据库驱动。
//   - 标准答案：评测场景的标准答案只有 chaos 和 eval 能读，其他代码读取它会报错，防止医生“偷看答案”。
//
// 它分析源码和医生的 Compose 挂载，不运行被检查的代码。运行时才能确定的文件路径，它发现不了，
// 所以它是第一道防线，不能当作安全隔离。
//
// 文件清单：
//   - doc.go：本包的使用背景、边界和阅读顺序。
//   - check.go：按目录、导入和文件访问规则检查架构边界。
//   - check_test.go：用故意违规的假模块检验架构规则。
//   - doctor.go：检查医生容器能从宿主机拿到什么。
//   - doctor_test.go：证明容器不能通过部署配置拿到故障答案或 Docker 控制权。
//   - main.go：把架构检查接到命令行与 CI。
//
// 阅读顺序：main.go → check.go → check_test.go。读 check.go 时，先看 locationProblem、
// importProblem、layerProblem，最后看较难的 scenarioProblems。
package main
