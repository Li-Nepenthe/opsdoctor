// Package inventoryservice 由库存进程入口调用，负责把 MySQL 仓储接到 gRPC 服务。端口开放前先完成迁移，停机时先排空库存请求，再释放数据库连接。
//
// config.go：读出库存和管理端口，与订单服务共用数据库连接池配置。
// app.go：建立连接、执行迁移，启动库存与管理端口，并等待在途请求结束。
package inventoryservice
