// Package inventoryservice 负责把库存服务组装起来并运行，从启动到退出的全过程都在这里。
//
// cmd/inventory-service/main.go 只调用这里的 Run。Run 依次做这些事：
//  1. 读取配置：数据库地址、gRPC 端口（默认 9001）、管理端口（默认 6062）。
//  2. 创建日志器，开始监听退出信号。
//  3. 连接 MySQL，执行迁移。
//  4. 组装库存业务（repository → service → gRPC 适配），注册到 gRPC 服务器。
//  5. 同时打开两个端口：gRPC 端口处理业务请求，管理端口提供健康检查。
//  6. 收到退出信号后，先关两个端口，等正在处理的请求做完；最后关数据库连接池。
//
// 为什么单独一个包：main 包没法测试，也不适合放复杂逻辑。组装代码放在这里，
// 库存的业务规则放在 internal/shop/inventory，两边互不混杂。
//
// 阶段 3 起，Run 还会初始化指标和调用链（telemetry 包），读的时候可以先跳过这部分。
//
// 文件清单：
//   - doc.go：本说明。
//   - config.go：库存服务的配置项，以及启动、停机各自的时限。
//   - app.go：Run 和 serve。
//
// 阅读顺序：config.go → app.go 的 Run（对照上面六步）→ serve。
package inventoryservice
