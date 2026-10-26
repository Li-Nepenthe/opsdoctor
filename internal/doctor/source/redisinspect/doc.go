// Package redisinspect 查看 Redis 的运行负担，不读取缓存内容。
//
// INFO 能回答连接有多少、内存用了多少、命中和未命中有多少。慢日志能说明哪些
// 命令执行得慢，但原始慢日志还可能带上业务 key 和 value，所以这里只留下命令名、
// 时间、耗时和参数数量。医生拿不到原参数。
//
// 文件清单：
//   - doc.go：本包的使用背景、边界和阅读顺序。
//   - client.go：通过独立 Redis ACL 账号读取运行信息。
//   - client_test.go：用一个只会回复固定消息的 Redis 协议小服务器检查适配器。
//   - permissions_integration_test.go：检查真实 Redis 的认证和只读账号权限。
//
// 阅读顺序：New → Info。账号权限由部署脚本设置，代码限制不会替代账号限制。
package redisinspect
