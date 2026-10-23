// Package inventoryservice 组装库存进程并拥有它创建的资源。
//
// cmd/inventory-service 调用 Run。本包接上配置、迁移、库存和通用服务器，不能把 SQL 或库存规则写进入口。
//
// 包内文件：
//   - doc.go：说明包的位置、边界和阅读路线。
//   - app.go：库存进程的资源组装与退出流程。
//   - config.go：库存进程自己的监听地址与启动关闭预算。
//
// 阅读顺序：config.go → app.go，再沿 inventory 的接口阅读。
package inventoryservice
