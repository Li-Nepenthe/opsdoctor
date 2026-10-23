// Package main 把目录分层和领域隔离变成可以在 CI 执行的检查。
//
// 开发者或 CI 运行本工具；它用标准库 AST 检查源码，不执行被扫描代码，也不是运行时访问沙箱。
//
// 包内文件：
//   - doc.go：说明包的位置、边界和阅读路线。
//   - check.go：按目录、导入和文件访问规则检查架构边界。
//   - check_test.go：用故意违规的假模块检验架构规则。
//   - main.go：把架构检查接到命令行与 CI。
//
// 阅读顺序：main.go → check.go → check_test.go。
package main
