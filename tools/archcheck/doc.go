// Package main 提供 archcheck，供 CI 检查仓库布局，以及医生能从源码和容器接触哪些文件和服务。
// 从 check.go 的 check 读起，看一次检查怎样遍历仓库。运行时才确定的路径要靠账号权限和部署隔离来限制。
//
//   - doc.go 说明 archcheck 负责的检查及其边界。
//   - main.go 把检查结果返回给命令行和 CI。
//   - check.go 检查文件位置和代码中能够推断出的场景读取路径。
//   - doctor.go 检查医生容器的挂载和权限，以及业务运维端口是否暴露给宿主机。
package main
