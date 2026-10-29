# internal/shop：靶场（电商业务）

这里是被医生诊断的那个系统：一个小型电商。它故意做成一个“正常的、写得认真的”后端，这样后面注入故障时，医生面对的才像真实的系统。

靶场不能导入医生（`doctor`）、故障注入（`chaos`）和评测（`eval`）的代码，它必须能单独运行。

## 里面有什么

| 包 | 作用 | 出现于 |
|---|---|---|
| [app](app/doc.go) | 各程序的组装代码：读配置、连依赖、启动服务、优雅停机。每个程序一个子目录 | 阶段 1 |
| [inventory](inventory/doc.go) | 库存：查询、扣减、归还 | 阶段 1 |
| [migrations](migrations/doc.go) | 五张表的建表 SQL | 阶段 1 |
| [seed](seed/doc.go) | 造测试数据：50 万商品、200 万订单 | 阶段 1 |
| [product](product/doc.go) | 商品查询，带 Redis 缓存；库存服务不可用时仍能返回商品信息 | 阶段 2 |
| [inventoryclient](inventoryclient/doc.go) | 订单服务调用库存服务的客户端，带超时、重试和熔断 | 阶段 2 |
| [order](order/doc.go) | 下单、查订单，以及防止重复下单的接口幂等 | 阶段 2 |
| [events](events/doc.go) | Kafka 消息：“订单已创建”消息的格式、发送和消费 | 阶段 2 |
| [sales](sales/doc.go) | 销量统计：消费订单消息并累加销量，同一条消息不会算两次 | 阶段 2 |

## 从哪里开始读

沿着一个请求读，不要逐个包翻：

- **阶段 1**：从 `app/inventoryservice` 进入，读到 `inventory`。
- **阶段 2 下单**：从 `app/orderservice` 进入，读 `order/service.go` 的 `Create`。
- **阶段 2 销量统计**：从 `app/orderworker` 进入，读 `events/consumer.go`，再到 `sales/repository.go`。

阶段 7 的 [ops](ops/doc.go) 提供内部运维接口：查看和临时调整连接池、预热少量商品缓存。它只认识 HTTP 请求和本系统运维令牌，不依赖医生；审批由医生完成，接口自己仍检查范围与操作编号。
