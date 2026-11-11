// Package mysqlinspect 读取指定数据库的执行计划、索引、语句摘要与锁等待，供医生定位慢请求。进程连接池和账号各限三条连接，排队也算在十秒查询预算内。
//
// 从 New 看诊断账号的连接限制，再看 Explain 如何只请求执行计划。
//
//   - client.go：通过受限连接池读取运行状态、执行计划和元数据。
//   - write_metadata.go：为审批和恢复核对列、索引以及旧 DDL 连接。
package mysqlinspect
