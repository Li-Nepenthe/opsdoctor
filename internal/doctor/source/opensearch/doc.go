// Package opensearch 将统一日志条件转成指定索引的 OpenSearch 只读搜索。服务名、级别等字段的部署差异由配置吸收，工具不能修改访问范围。
//
// 从 Config 看后端与索引如何绑定，再沿 Search 查看条件怎样编码、结果怎样校验。请求拒绝重定向，响应有大小上限，启用认证时使用只读账号。
//
//   - config.go：校验后端地址、索引范围和字段映射。
//   - client.go：编码查询条件并读回固定索引的搜索响应。
//   - response.go：校验查询完整性，识别日志时间、来源和消息。
package opensearch
