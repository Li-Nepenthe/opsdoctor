// Package jaeger 把 Jaeger 的调用链转换成医生认识的步骤和父子关系。
//
// 一次下单会经过几个服务。每个服务记录自己做的一小步，这些步骤叫 span。
// Jaeger 把服务资料放在 processes 中，让 span 通过编号引用它。这里先把两者合并，
// 工具才能直接读到“库存服务的 SQL 花了多久”，而不是只看到一个 process ID。
//
// 当前固定 Jaeger 2.20.0，使用实测的 /api/traces 搜索与详情接口。这个内部 JSON
// 接口不承诺跨版本兼容，因此升级时必须同时回归医生适配器，不能只检查 Grafana。
// 有些部署会把入口放在 /jaeger/ui 之下；本包只接受普通路径段作为固定前缀，
// 编码路径、点路径和重定向仍被拒绝，工具不能借前缀修改后端或 API 路径。
//
// 文件清单：
//   - doc.go：本包的使用背景、边界和阅读顺序。
//   - client.go：读取本项目固定的 Jaeger 2.20.0 查询接口。
//   - client_test.go：用真实样例检查根地址、部署前缀、搜索详情契约及异常边界。
//
// 阅读顺序：wireTrace → decode → Search → Get。
package jaeger
