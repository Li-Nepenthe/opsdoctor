# 靶场业务

当前实现库存、订单、商品缓存、消息消费与数据基线；不能依赖 doctor、chaos 或 eval。不提前建立后续阶段的空包。

目录内容：

- [app](app/doc.go)：共用配置，以及三个服务和 shopctl 的组装。
- [inventory](inventory/doc.go)：模型、业务、MySQL 与 gRPC 适配。
- [inventoryclient](inventoryclient/doc.go)：带超时、重试和熔断的库存客户端。
- [product](product/doc.go)：商品缓存、价格查询和库存展示降级。
- [order](order/doc.go)：下单、幂等状态、补偿与订单查询。
- [events](events/doc.go)：订单消息契约、Kafka 发送和手动位点提交。
- [sales](sales/doc.go)：事务去重、销量累加与总量核对。
- [migrations](migrations/doc.go)：五张表和嵌入入口。
- [seed](seed/doc.go)：批量建立历史基线。

阅读建议：从对应 app 进入；先读 service 需要的接口，再读 repository、cache 和消息适配。阶段 2 下单从 app/orderservice 到 order/service.go，销量消费从 app/orderworker 到 events/consumer.go 再到 sales/repository.go。
