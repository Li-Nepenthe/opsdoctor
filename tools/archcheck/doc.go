// Package main 是 archcheck 工具：检查文件位置、标准答案读取和医生的 Compose 配置。
//
// 项目对“代码放在哪里、谁能依赖谁”有一些规定（计划书第 5.1 节）。这些规定靠人记容易忘，
// 所以在 CI 里自动检查。导入方向和文件角色已经交给 depguard，规则在根目录 .golangci.yml，
// 使用 golangci-lint run ./... 执行；另加 --build-tags=leakbug 检查故障构建版本。
// archcheck 只保留 depguard 不处理的三类项目约束：
//   - 位置：根目录不能放 .go 文件；cmd/<程序>/ 下只能有一个 main.go；包名不能太含糊。
//   - 标准答案：评测场景的标准答案只有 chaos 和 eval 能读，其他代码读取它会报错，防止医生“偷看答案”。
//   - Compose：医生不能挂载故障答案或 Docker 控制入口，内部服务端口也不能暴露给宿主机。
//
// 它分析源码和医生的 Compose 挂载，不运行被检查的代码。运行时才能确定的文件路径，它发现不了，
// 所以它是第一道防线，不能当作安全隔离。
//
// 文件清单：
//   - doc.go：本包的使用背景、边界和阅读顺序。
//   - check.go：检查文件位置和标准答案读取，并接入 Compose 边界检查。
//   - check_test.go：用临时源码检验文件位置和标准答案读取规则。
//   - doctor.go：检查医生容器能从宿主机拿到什么。
//   - doctor_test.go：证明容器不能通过部署配置拿到故障答案或 Docker 控制权。
//   - main.go：把架构检查接到命令行与 CI。
//
// 阅读顺序：main.go → check.go → check_test.go。读 check.go 时，先看 locationProblem，
// 再看较难的 scenarioProblems；容器边界单独看 doctor.go 和 doctor_test.go。
package main
