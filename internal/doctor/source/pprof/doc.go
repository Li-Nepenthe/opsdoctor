// Package pprof 按堆栈统计各服务的 goroutine 停在哪里。调用者只能选择已经配置的服务，不能提供任意管理地址或选择其他 profile。
//
// 从 Goroutines 查看服务绑定，再看 parse 怎样忽略内存地址来合并相同调用栈。
//
//   - client.go：读取固定的 goroutine 文本端点，合并堆栈并按数量排序。
package pprof
