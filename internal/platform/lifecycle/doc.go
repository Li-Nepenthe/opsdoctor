// Package lifecycle 负责服务的“优雅停机”：收到退出信号后，先让手上的工作收尾，再按顺序关闭资源。
//
// 为什么需要它：直接杀掉进程，正在处理的请求会被打断，数据库连接也来不及正常关闭。
// 优雅停机分两步，本包各提供一个函数：
//  1. Notify：把退出信号变成 context 的取消通知。你在终端按 Ctrl+C，或者 Docker 停止容器时
//     发送 SIGTERM，Notify 返回的 context 就会被取消，所有使用它的地方都会收到“该停了”的通知。
//  2. Shutdown：按 app 给定的顺序逐个关闭资源。比如先关 HTTP 服务器，让正在处理的请求做完，
//     再关数据库连接池。所有关闭步骤共用一个总时限，超时就不再等。
//
// 用法：每个服务的 app 在启动时调用 Notify，退出时调用 Shutdown，并传入一组 Hook，每个 Hook 是一个关闭步骤。
// 本包不知道 HTTP、gRPC、数据库具体怎么关，这些写在 app 提供的 Hook 里。
//
// 文件清单：
//   - doc.go：本说明。
//   - lifecycle.go：Notify、Shutdown 和 Hook。
//   - lifecycle_test.go：检查关闭顺序、总时限，以及父 context 取消时 Notify 是否跟着取消。
//
// 阅读顺序：先读 Notify（只有一行），再读 Shutdown。重点看它为什么用 context.WithoutCancel
// 给清理另开一个时限，而不是直接用已经被取消的 context。
package lifecycle
