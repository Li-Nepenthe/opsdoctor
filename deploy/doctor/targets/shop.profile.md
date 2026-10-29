# shop 的系统说明书

shop 是一个小型电商系统。用户可以查商品、创建订单、查询订单详情，以及按用户分页查询订单。

## 服务与调用关系

- order-service 提供 HTTP 接口。`GET /products/:id` 查询商品；`POST /orders` 创建订单；`GET /orders/:id` 查询详情；`GET /orders` 按用户分页查询。
- inventory-service 提供 gRPC 的 GetStock、Deduct 和 Release。订单服务通过它查询实时库存、扣减库存和归还库存。
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

商品缓存键使用 `product:<商品ID>`，正常有效期为十分钟加零到一分钟的随机增量。不存在的商品使用短期空值缓存。同一个商品的同时回源可以在进程内合并成一次查询。

下单幂等记录使用 `idempotency:<幂等键>`，默认保存二十四小时。记录区分处理中、已完成与结果待核对。结果待核对时不自动重做。Redis 使用 DB 0，开启 AOF 持久化和 noeviction，避免淘汰尚在承诺期内的幂等记录。

`order.created` 消息包含事件编号、订单号、商品编号、数量与时间。订单事务与消息发送不在同一个数据库事务中。订单已经提交但发送结果不确定时，服务会记录告警日志。

## 可观测数据

三个业务进程输出带 service、request_id、trace_id 等字段的 JSON 日志。Prometheus 保存接口耗时、数据库连接池、缓存、RPC 和消息等指标。Jaeger 保存调用链。Loki 保存日志。MySQL、Redis 和 Kafka exporter 提供中间件指标。

三个业务进程的管理端口提供 `/metrics`、`/health` 和 Go 的 goroutine 堆栈接口。医生的地址由接入配置固定，不从工具参数中读取。

## 时间预算与一致性

库存查询有总超时预算，只对可安全重试的查询进行有限重试。扣减不会自动重试。商品详情在库存查询失败时仍可返回商品信息，并把库存标为暂不可用。订单已经提交后的消息发送使用独立且有限的收尾预算。

调用链和日志是观测到的片段，采样或保留时间可能使部分请求缺失。判断一次业务操作是否完成，应结合数据库状态、消息消费和日志，不能单凭没有查到调用链就下结论。

## 报告中的组件标识

组件标识用于把自然语言结论对应到架构中的一个位置。服务直接用服务名；服务内的模块用“服务名:模块名”；表用“mysql:库名.表名”；调用链路用“调用方->被调用方”。选择证据能支持的最具体位置。尚不能定位时用 unknown，并在 unknowns 说明还缺什么。表里不表示哪个组件更容易出问题。

下面是全部可选值。表的两端标记供程序读取，新增架构组件时一起维护这张表。

<!-- COMPONENTS_START -->
| 标识 | 架构职责 |
|---|---|
| `unknown` | 证据尚不足以定位到架构中的某处 |
| `none` | 已检查的范围内没有发现故障；需要在报告里说明观察时间和覆盖范围 |
| `order-service` | HTTP 业务服务整体 |
| `inventory-service` | 库存 gRPC 服务整体 |
| `order-worker` | 消息消费与销量统计进程整体 |
| `order-service:http` | 路由、参数解析和 HTTP 响应 |
| `order-service:rate_limit` | 限制进入服务的请求速率 |
| `order-service:response_processing` | 业务处理完成后的响应收尾 |
| `order-service:product` | 商品详情业务 |
| `order-service:product_cache` | 商品信息缓存、空值、有效期和同键查询合并 |
| `order-service:inventory_client` | 库存调用、时限、重试、熔断与降级 |
| `order-service:order` | 下单和订单查询业务 |
| `order-service:idempotency` | 下单的重复请求与结果记录 |
| `order-service:database_pool` | 订单与商品 SQL 复用的连接池 |
| `order-service:kafka_producer` | 发送已创建订单的消息 |
| `order-service:telemetry` | 指标、调用链和日志记录 |
| `inventory-service:grpc` | gRPC 参数与状态码转换 |
| `inventory-service:inventory` | 查询、扣减和归还库存的业务 |
| `inventory-service:database_pool` | 库存 SQL 复用的连接池 |
| `inventory-service:telemetry` | 指标、调用链和日志记录 |
| `order-worker:kafka_consumer` | 拉取消息、分区消费和提交位点 |
| `order-worker:sales` | 事务内去重并累计销量 |
| `order-worker:database_pool` | 消费事务复用的连接池 |
| `order-worker:telemetry` | 指标、调用链和日志记录 |
| `mysql` | shop 使用的 MySQL 实例 |
| `mysql:shop.products` | 商品基础信息表 |
| `mysql:shop.stock` | 库存表 |
| `mysql:shop.orders` | 订单表 |
| `mysql:shop.product_sales` | 累计销量表 |
| `mysql:shop.processed_events` | 已处理消息表 |
| `redis` | 商品缓存和幂等记录的存储 |
| `kafka` | 订单消息的存储与分发 |
| `kafka:order.created` | 订单创建消息主题 |
| `order-service->inventory-service` | HTTP 服务到库存 gRPC 服务的调用链路 |
| `order-service->mysql` | 商品与订单 SQL 的连接链路 |
| `inventory-service->mysql` | 库存 SQL 的连接链路 |
| `order-worker->mysql` | 消费事务的连接链路 |
| `order-service->redis` | 商品缓存与幂等操作的连接链路 |
| `order-service->kafka` | 消息发送链路 |
| `kafka->order-worker` | 消息消费链路 |
| `loadgen` | 以固定工作者发出业务请求的流量生成器 |
| `prometheus` | 指标采集、存储和告警规则计算 |
| `alertmanager` | 告警分组与通知发送 |
| `jaeger` | 调用链采集与查询 |
| `loki` | 日志存储与查询 |
| `alloy` | 容器日志采集 |
| `grafana` | 观测数据展示 |
| `mysqld-exporter` | MySQL 运行状态采集 |
| `redis-exporter` | Redis 运行状态采集 |
| `kafka-exporter` | Kafka 分区与消费延迟采集 |
<!-- COMPONENTS_END -->
