// Package mock 让没有模型账号的开发环境也能完整演示诊断流程。
//
// 它按照固定脚本读取架构和告警，再交一份明确标成演示的报告。它不会分析真实
// 根因，也不会联网。每轮由请求里的历史决定下一步，因此并发诊断不会共用一个计数器。
//
// 文件清单：doc.go 是本说明；client.go 实现固定脚本。
// 阅读顺序：client.go 的 Complete → observations。
package mock
