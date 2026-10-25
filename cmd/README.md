# cmd：程序入口

这里放项目里每一个能运行的程序，一个子目录就是一个程序。

每个子目录里只有一个 `main.go`，而且非常短：它只调用 internal 里对应包的 `Run` 函数。真正的启动逻辑，比如读配置、连数据库、启动服务器，都写在那个包里。

为什么这样分：`main` 包不能被别的代码导入，也就没法写测试。把逻辑放进普通的包，就能测试；`main.go` 只剩几行，打开一眼就能看懂。

## 里面有什么

| 程序 | 作用 | 出现于 |
|---|---|---|
| [inventory-service](inventory-service/main.go) | 库存服务：提供扣库存、归还库存、查库存三个 gRPC 接口 | 阶段 1 |
| [shopctl](shopctl/main.go) | 命令行工具：造测试数据、手动查库存和扣库存、核对订单和销量 | 阶段 1 |
| [order-service](order-service/main.go) | 订单服务：对外提供商品查询、下单、查订单的 HTTP 接口 | 阶段 2 |
| [order-worker](order-worker/main.go) | 从 Kafka 读取“订单已创建”的消息，累加商品销量 | 阶段 2 |
| [loadgen](loadgen/main.go) | 压测工具：按设定的速率持续发请求，可以通过控制接口随时调整速率 | 阶段 2 |
| [alertsink](alertsink/main.go) | 临时的告警接收器，把收到的告警写进日志。阶段 5 由医生取代 | 阶段 3 |
| [chaos](chaos/main.go) | 宿主机实验命令：列出场景、注入、恢复、自动检查和整套自测 | 阶段 4 |

## 从哪里开始读

打开某个程序的 `main.go`，找到它调用的 `Run`，跟着进去读。例如 `inventory-service/main.go` 调用的是 `inventoryservice.Run`，它在 [internal/shop/app/inventoryservice](../internal/shop/app/inventoryservice/doc.go) 里。
