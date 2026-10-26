# shop 的系统说明书

shop 是一个小型电商系统。用户可以查商品、创建订单、查询订单详情，以及按用户分页查询订单。

## 服务与调用关系

- order-service 提供 HTTP 接口。`GET /products/:id` 查询商品；`POST /orders` 创建订单；`GET /orders/:id` 查询详情；`GET /orders` 按用户分页查询。
- inventory-service 提供 gRPC 的 GetStock、Deduct 和 Return。订单服务通过它查询实时库存、扣减库存和归还库存。
- order-worker 消费 Kafka 的 `order.created` 消息，在 MySQL 中更新商品累计销量。消费组名是 `order-worker`。
- 商品查询先读 Redis，未命中时由 order-service 查询 MySQL，再把结果写入缓存。商品缓存不保存实时库存；实时库存通过 gRPC 单独查询。
- 创建订单时，order-service 先通过库存服务扣减，再写入订单，最后发送消息。消息由 worker 异步消费。

## MySQL 表

业务库名为 `shop`。五张表的作用如下：

| 表 | 内容与主要查找方式 |
|---|---|
| products | 商品名称、价格等基本信息，按商品 ID 查询 |
| stock | 每个商品的可用库存，按 sku_id 查询和更新；sku_id 的唯一索引用于定位对应商品行 |
| orders | 订单、用户、商品、数量与金额；主键用于查详情，user_id 与 created_at 的联合索引用于按用户取时间倒序的订单页 |
| product_sales | 按商品累计的已消费销量，sku_id 用于定位计数行 |
| processed_events | 已处理消息的 ID，用唯一约束避免同一消息重复计入销量 |

## Redis 与消息

商品缓存键使用 `product:<商品ID>`，正常有效期为五分钟加零到一分钟的随机增量。不存在的商品使用短期空值缓存。同一个商品的同时回源可以在进程内合并成一次查询。

下单幂等记录使用 `idempotency:<幂等键>`，默认保存二十四小时。记录区分处理中、已完成与结果待核对。结果待核对时不自动重做。Redis 使用 DB 0，开启 AOF 持久化和 noeviction，避免淘汰尚在承诺期内的幂等记录。

`order.created` 消息包含事件编号、订单号、商品编号、数量与时间。订单事务与消息发送不在同一个数据库事务中。订单已经提交但发送结果不确定时，服务会记录告警日志。

## 可观测数据

三个业务进程输出带 service、request_id、trace_id 等字段的 JSON 日志。Prometheus 保存接口耗时、数据库连接池、缓存、RPC 和消息等指标。Jaeger 保存调用链。Loki 保存日志。MySQL、Redis 和 Kafka exporter 提供中间件指标。

三个业务进程的管理端口提供 `/metrics`、`/health` 和 Go 的 goroutine 堆栈接口。医生的地址由接入配置固定，不从工具参数中读取。

## 时间预算与一致性

库存查询有总超时预算，只对可安全重试的查询进行有限重试。扣减不会自动重试。商品详情在库存查询失败时仍可返回商品信息，并把库存标为暂不可用。订单已经提交后的消息发送使用独立且有限的收尾预算。

调用链和日志是观测到的片段，采样或保留时间可能使部分请求缺失。判断一次业务操作是否完成，应结合数据库状态、消息消费和日志，不能单凭没有查到调用链就下结论。
