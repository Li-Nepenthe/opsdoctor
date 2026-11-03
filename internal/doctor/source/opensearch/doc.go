// Package opensearch 把统一日志条件翻译成 OpenSearch 的只读搜索。
//
// 工具想查“哪个服务在这段时间报了什么错”，不需要知道日志索引叫什么、字段怎样命名。
// New 在启动时固定这些部署信息；Search 只接受 source.LogQuery，并把字符串交给 JSON
// 编码器处理。关键字因此只是查询值，不能变成另一段搜索语句。
//
// 这里直接使用 net/http，不引入产品客户端。HTTP 请求只会访问指定索引的 _search，
// 不跟随重定向，响应有大小上限。开启安全插件时，部署者应提供只有读权限的账号。
//
// 文件清单：
//   - doc.go：说明统一日志接口与本适配器的关系和阅读顺序。
//   - config.go：校验根地址、索引范围和字段映射。
//   - client.go：发送有界搜索请求，并把结构化条件变成查询 DSL。
//   - response.go：读取不同形状的字段，把时间统一成 Unix 纳秒。
//   - client_test.go：用本地后端验证查询值、响应边界和只读请求路径。
//   - integration_test.go：显式设置 TEST_OPENSEARCH_* 后查询真实日志，平时跳过。
//
// 阅读顺序：source.LogQuery → Config → expression → Search → entries。
package opensearch
