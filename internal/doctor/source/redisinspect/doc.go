// Package redisinspect 查看 Redis 的连接、内存和慢命令。客户端只读取固定分区或慢日志摘要，部署时还要限制账号的 ACL。
//
// 从 New 查看账号和连接限制，再看 Info 如何在慢日志中去掉可能含有业务 key、value 的参数。
//
//   - client.go：通过受限账号读取 INFO 或不含原始参数的慢日志摘要。
package redisinspect
