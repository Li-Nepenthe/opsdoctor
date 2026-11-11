// Package main 提供 commentcheck。CI 用它定位缺失的包说明、文件说明和导出声明的注释。
// 本地也可用同一份扫描结果生成阅读清单（-list）和代码地图（-map）。从 main.go 的 run 读起。
//
//   - main.go：处理命令行选项，将检查结果交给终端或 CI。
//   - check.go：检查包说明和导出声明，同时记录所有类型、函数和方法的位置。
//   - files.go：核对文件开头的说明与 doc.go 的清单，生成从 docs 目录可打开的文件链接。
package main
