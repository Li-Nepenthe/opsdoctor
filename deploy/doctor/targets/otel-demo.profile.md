# otel-demo 的系统说明书

otel-demo 是 OpenTelemetry Demo 的 Astronomy Shop，一个展示望远镜等商品的电商示例。用户可以浏览商品、获取推荐和广告、管理购物车，并填写地址和支付信息完成结账。本说明依据上游 3.1.0 的公开架构与源码整理。

## 服务与调用关系

- frontend-proxy 使用 Envoy 接收浏览器请求，把商店请求转给 frontend，并提供观测界面的代理入口。image-provider 提供商品图片。
- frontend 是 Next.js 应用，同时提供网页和 `/api/*` 接口。它调用 product-catalog 查询商品、recommendation 查询推荐、ad 获取广告、cart 管理购物车、currency 换算货币，以及 checkout 完成结账。
- product-catalog 是 Go gRPC 服务，提供 ListProducts、GetProduct 和 SearchProducts，从 PostgreSQL 读取商品信息。
- cart 是 .NET gRPC 服务，提供 AddItem、GetCart 和 EmptyCart，使用 Valkey 保存每个用户的购物车。
- checkout 是 Go gRPC 服务，提供 PlaceOrder。它读取购物车、查询商品价格和运费、换算金额、请求 payment 支付、请求 shipping 发货、清空购物车，并发送确认邮件。
- payment 是 Node.js gRPC 服务，处理 Charge 请求。shipping 是 Rust HTTP 服务，计算运费并生成跟踪编号；它使用 PHP 的 quote 服务计算报价。email 是 Ruby HTTP 服务，生成订单确认邮件。
- currency 是 C++ gRPC 服务；ad 是 Java gRPC 服务；recommendation 是 Python gRPC 服务。
- checkout 向 Kafka 的 `orders` 主题发送订单。accounting 消费订单并写入 PostgreSQL；fraud-detection 是另一个订单消费者。
- load-generator 使用 Locust 模拟浏览、查询推荐与广告、购物车和结账等客户端行为。

## 存储与消息

PostgreSQL 实例名为 astronomy-db，业务库为 astronomy_db。`catalog.products` 保存商品的名称、说明、图片、价格和分类。`accounting` 模式中的 `order`、`orderitem` 和 `shipping` 分别保存订单编号、商品明细和运输信息。

Valkey 实例名为 valkey-cart，保存用户购物车。Kafka 的 `orders` 主题连接结账服务和异步消费者；同步结账调用与异步记账是两条不同的处理路径。

## 可观测数据与查询范围

各服务使用 OpenTelemetry 产生调用链、指标和日志，由 otel-collector 接收。调用链送到 Jaeger，指标送到 Prometheus，日志送到 OpenSearch。Grafana 展示这些数据。服务名使用 `frontend`、`checkout`、`product-catalog` 等上游名称。

Prometheus 中由资源属性提升的服务标签为 `service_name`。Collector 也根据 span 生成调用次数和耗时指标。不同语言的 SDK 可能使用不同的原生 HTTP、RPC 指标名称和单位；应先查询实际存在的指标，不把缺少某个名称直接视为服务没有流量。

本次接入提供 Prometheus、Jaeger 和 OpenSearch 查询。没有配置 PostgreSQL 的 SQL、索引或锁查询适配器，也没有配置 Go pprof、Redis 只读账号或业务写操作。相应工具不会提供给本系统；可用证据以指标、调用链和日志为准。Prometheus 中的中间件指标与直接检查数据库或 Valkey 的能力不同。

调用链保存在 Jaeger 内存中，日志和指标也都有保留范围。没有找到记录时，应说明查询的时间、服务和覆盖范围，再判断是采集缺失、采样、时间范围还是业务本身没有执行。

## 报告中的组件标识

服务整体使用服务名，调用链路使用“调用方->被调用方”，数据库表使用“postgresql:库名.模式名.表名”。请选择证据能够支持的最具体位置；不能定位时使用 unknown，并说明缺少的证据。下表只说明架构职责。

<!-- COMPONENTS_START -->
| 标识 | 架构职责 |
|---|---|
| `unknown` | 证据尚不足以定位到某个架构位置 |
| `none` | 本次已检查的时间和范围内没有发现故障 |
| `frontend-proxy` | Envoy HTTP 入口和请求转发 |
| `frontend` | 商店网页与 API 接口 |
| `image-provider` | 商品图片服务 |
| `product-catalog` | 商品列表、详情和搜索 |
| `recommendation` | 商品推荐服务 |
| `ad` | 广告查询服务 |
| `cart` | 用户购物车服务 |
| `checkout` | 结账流程的同步编排 |
| `payment` | 支付请求处理 |
| `shipping` | 运费查询、发货和跟踪编号生成 |
| `quote` | 运费报价计算 |
| `currency` | 金额的币种换算 |
| `email` | 订单确认邮件 |
| `accounting` | 消费订单并记录账务信息 |
| `fraud-detection` | 订单事件的风控消费者 |
| `valkey-cart` | 用户购物车存储 |
| `astronomy-db` | PostgreSQL 业务数据库实例 |
| `postgresql:astronomy_db.catalog.products` | 商品信息表 |
| `postgresql:astronomy_db.accounting.order` | 订单编号表 |
| `postgresql:astronomy_db.accounting.orderitem` | 订单商品明细表 |
| `postgresql:astronomy_db.accounting.shipping` | 订单运输信息表 |
| `kafka` | 订单消息存储和分发 |
| `kafka:orders` | 订单事件主题 |
| `frontend-proxy->frontend` | 入口到商店网页和 API 的调用链路 |
| `frontend->product-catalog` | 网页到商品服务的调用链路 |
| `frontend->recommendation` | 网页到推荐服务的调用链路 |
| `frontend->ad` | 网页到广告服务的调用链路 |
| `frontend->cart` | 网页到购物车服务的调用链路 |
| `frontend->checkout` | 网页到结账服务的调用链路 |
| `checkout->cart` | 结账读取和清空购物车的调用链路 |
| `checkout->product-catalog` | 结账查询商品信息的调用链路 |
| `checkout->currency` | 结账换算金额的调用链路 |
| `checkout->payment` | 结账请求支付的调用链路 |
| `checkout->shipping` | 结账请求运费和发货的调用链路 |
| `checkout->email` | 结账发送确认邮件的调用链路 |
| `checkout->kafka` | 结账发送订单事件的链路 |
| `shipping->quote` | 发货服务获取报价的链路 |
| `cart->valkey-cart` | 购物车服务访问存储的链路 |
| `product-catalog->astronomy-db` | 商品服务访问 PostgreSQL 的链路 |
| `kafka->accounting` | 订单消息到记账消费者的链路 |
| `kafka->fraud-detection` | 订单消息到风控消费者的链路 |
| `accounting->astronomy-db` | 记账消费者写入 PostgreSQL 的链路 |
| `load-generator` | 发出合成业务请求的客户端 |
| `otel-collector` | 接收、处理和转发观测信号 |
| `prometheus` | 指标存储与查询 |
| `jaeger` | 调用链存储与查询 |
| `opensearch` | 日志存储与查询 |
| `grafana` | 观测数据展示 |
| `opamp-server` | 接收观测组件的状态信息 |
<!-- COMPONENTS_END -->

## 架构资料

- [官方架构](https://opentelemetry.io/docs/demo/architecture/)
- [3.1.0 核心服务部署定义](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/compose.yaml)
- [3.1.0 消息及消费者部署定义](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/compose.full.yaml)
- [3.1.0 PostgreSQL 表结构](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/src/postgresql/init.sql)
