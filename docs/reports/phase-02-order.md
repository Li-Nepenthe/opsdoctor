# 阶段 2 汇报书：订单服务、缓存与消息

日期：2026-10-23。分支：`phase-02-order`。基线 main：`3970868dae0e298c3cb0b1269a6800a473e2e7a7`。本地工程验收与 [Linux CI](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/39961041773) 均通过，记录见[进度看板](../../进度.md)和[实测记录](../notes.md#phase-02)。

## 一、总览

订单、缓存、消息消费和流量工具已完成，六项本地工程验收通过。

本阶段提供四个订单与商品 HTTP 接口，复用库存 gRPC。商品查询使用 Redis 缓存和单进程请求合并，库存展示可降级。订单创建后向 Kafka 发送消息，消费者在 MySQL 事务内去重并累加销量。入口限流、查询重试和库存调用熔断已经接入；不确定下单结果保留幂等记录，记录日志和用户指定的计数。

| 计划中的“做” | 状态 | 交付 |
|---|---|---|
| 订单服务全部 HTTP 接口 | 已完成 | 商品详情、下单、订单详情、按用户分页 |
| 商品缓存 | 已完成 | Cache Aside、空值、TTL 抖动、可关闭的 singleflight |
| 下单与补偿 | 已完成 | 查价、扣减、事务、发消息；确认订单未提交才补偿 |
| Kafka 消费者 | 已完成 | 消费组、事务去重、手动提交、当前消息收尾 |
| loadgen | 已完成 | 70/20/10、Zipf、控制接口、十秒与全程真实统计 |
| shopctl check | 已完成 | 只读总量核对，差值非零返回失败并解释原因 |
| 服务治理 | 已完成 | 限流、共享超时、查询退避重试、独立熔断与展示降级 |
| 下单幂等 | 已完成 | 同键回放、处理中与待核对分开、保守失败边界、日志和计数 |

## 二、新引入的技术与依赖

| 技术或库 | 版本 | 用在哪里 | 为什么选它 | 放弃的替代方案 |
|---|---|---|---|---|
| go-redis/v9 | v9.23.0 | 商品缓存与幂等记录 | 计划指定的 Redis 客户端 | 自己实现 RESP |
| kafka-go | v0.4.51 | 同步发送、消费组与手动位点 | 计划指定，纯 Go 客户端 | 改用其他消息系统 |
| gobreaker/v2 | v2.4.0 | GetStock 与 Deduct 独立熔断 | 计划指定，状态转换由库处理 | 自写并发状态机 |
| x/time | v0.16.0 | 令牌桶 | 计划指定 | 在业务 handler 中手写计数 |
| x/sync | v0.24.0 | singleflight | 使用已有同 key 合并工具 | 自己维护等待队列 |
| prometheus/client_golang | v1.25.0 | 用户要求的 needs_review 计数 | 计划内组件，用户明确提前要求这一项 | 提前展开整个阶段 3 |

Go 1.27.1、MySQL 8.4.13、Redis 8.10.2、Kafka 4.4.0、golang:1.27.1-alpine3.24、alpine:3.24.2、staticcheck v0.8.2 和固定 ubuntu-24.04 沿用已有版本。间接依赖以 go.mod/go.sum 为准，未为业务引入额外测试框架。版本与取舍见 [decisions](../decisions.md)。

## 三、代码明细

### 3.1 包与职责

| 包路径 | 新增 / 修改 | 职责 | 对外提供什么 |
|---|---|---|---|
| [cmd/loadgen](../../cmd/loadgen/main.go) | 新增 | // 压测入口只传递参数、环境读取器与输出；调度和控制端口由 loadgen.Run 管理。 | main 作为独立命令入口 |
| [cmd/order-service](../../cmd/order-service/main.go) | 新增 | // 订单服务入口只转换退出码；资源装配、信号处理和请求排空由 orderservice.Run 完成。 | main 作为独立命令入口 |
| [cmd/order-worker](../../cmd/order-worker/main.go) | 新增 | // 订单消费者入口只负责退出码，连接和停机顺序由 orderworker.Run 管理。 | main 作为独立命令入口 |
| [internal/doctor/auth](../../internal/doctor/auth/doc.go) | 修改 | auth 医生认证的凭据规则，目前还没有接到 HTTP。 | 原有接口不变；本阶段只整理说明和注释 |
| [internal/loadgen](../../internal/loadgen/doc.go) | 新增 | loadgen 为靶场持续生成真实 HTTP 请求。 | Run；内部调度、控制与统计 |
| [internal/platform/adminserver](../../internal/platform/adminserver/doc.go) | 修改 | adminserver 提供独立管理端口的健康检查。 | 原有接口不变；本阶段只整理说明和注释 |
| [internal/platform/config](../../internal/platform/config/doc.go) | 修改 | config 把环境配置转成启动前可以信任的值。 | 原有接口不变；本阶段只整理说明和注释 |
| [internal/platform/grpcx](../../internal/platform/grpcx/doc.go) | 修改 | grpcx 处理一元 RPC 的通用拦截和有界停机。 | 原有接口不变；本阶段只整理说明和注释 |
| [internal/platform/httpx](../../internal/platform/httpx/doc.go) | 修改 | httpx 集中处理 HTTP 入口的通用保护与统一响应。 | 原有接口不变；本阶段只整理说明和注释 |
| [internal/platform/lifecycle](../../internal/platform/lifecycle/doc.go) | 修改 | lifecycle 将退出信号转换成取消通知，再按顺序清理资源。 | 原有接口不变；本阶段只整理说明和注释 |
| [internal/platform/logging](../../internal/platform/logging/doc.go) | 修改 | logging 把服务与请求关联信息加入 JSON 日志。 | 原有接口不变；本阶段只整理说明和注释 |
| [internal/platform/migrate](../../internal/platform/migrate/doc.go) | 修改 | migrate 在同一数据库会话持锁执行嵌入式迁移。 | 原有接口不变；本阶段只整理说明和注释 |
| [internal/platform/mysqlx](../../internal/platform/mysqlx/doc.go) | 修改 | mysqlx 建立配置一致的 MySQL 连接池。 | 原有接口不变；本阶段只整理说明和注释 |
| [internal/platform/resilience](../../internal/platform/resilience/doc.go) | 新增 | resilience 给外部调用加上限流、重试和熔断保护。 | RateLimit、RequestBudget、NewRetrier、NewBreaker |
| [internal/shop/app](../../internal/shop/app/doc.go) | 修改 | app 保存 shop 程序共同的数据库配置读取步骤。 | 原有接口不变；本阶段只整理说明和注释 |
| [internal/shop/app/inventoryservice](../../internal/shop/app/inventoryservice/doc.go) | 修改 | inventoryservice 组装库存进程并拥有它创建的资源。 | 原有接口不变；本阶段只整理说明和注释 |
| [internal/shop/app/orderservice](../../internal/shop/app/orderservice/doc.go) | 新增 | orderservice 组装订单 HTTP 进程。 | Run，组装订单进程 |
| [internal/shop/app/orderworker](../../internal/shop/app/orderworker/doc.go) | 新增 | orderworker 组装订单消息消费者进程。 | Run，组装消费进程 |
| [internal/shop/app/shopctl](../../internal/shop/app/shopctl/doc.go) | 修改 | shopctl 组装宿主机造数与库存调试命令。 | Run 新增 check 分派 |
| [internal/shop/events](../../internal/shop/events/doc.go) | 新增 | events 定义订单创建消息，并负责它在 Kafka 中的发送与接收。 | OrderCreated、Publisher、Consumer、EnsureTopic |
| [internal/shop/inventory](../../internal/shop/inventory/doc.go) | 修改 | inventory 实现库存查询、扣减和归还，分开业务、数据与传输职责。 | 原有接口不变；本阶段只整理说明和注释 |
| [internal/shop/inventoryclient](../../internal/shop/inventoryclient/doc.go) | 新增 | inventoryclient 把商品和订单需要的库存操作转换成 gRPC 调用。 | Client 的查询、扣减、归还 |
| [internal/shop/migrations](../../internal/shop/migrations/doc.go) | 修改 | migrations 保存 shop 五张表与嵌入入口。 | 原有接口不变；本阶段只整理说明和注释 |
| [internal/shop/order](../../internal/shop/order/doc.go) | 新增 | order 负责下单、订单查询和请求去重。 | Service、Repository、IdempotencyStore、Handler |
| [internal/shop/product](../../internal/shop/product/doc.go) | 新增 | product 负责商品价格查询和商品详情。 | Service、RedisCache、MySQLRepository、Handler |
| [internal/shop/sales](../../internal/shop/sales/doc.go) | 新增 | sales 把订单消息累加成商品销量，并提供订单与销量的总量核对。 | Service.Handle、MySQLRepository.Apply/Check |
| [internal/shop/seed](../../internal/shop/seed/doc.go) | 修改 | seed 建立靶场的商品、库存与历史订单基线。 | 原有接口不变；本阶段只整理说明和注释 |
| [tools/archcheck](../../tools/archcheck/doc.go) | 修改 | main 把目录分层和领域隔离变成可以在 CI 执行的检查。 | 原有接口不变；本阶段只整理说明和注释 |
| [tools/commentcheck](../../tools/commentcheck/doc.go) | 修改 | main 把目录、文件、代码三层说明的可检查部分接到 CI。 | 原有接口不变；本阶段只整理说明和注释 |

### 3.2 文件明细

下面以本分支相对 main 的变更生成，包含代码、注释、测试、配置与文档。旧阶段文件的改动主要是补齐“主要内容”，不重写历史报告的当时行号。

<!-- FILES_START -->
| 文件 | 当前行数 | + / − | 作用 |
|---|---:|---:|---|
| [.env.example](../../.env.example) | 77 | +32 / −2 | 正式配置、隔离测试地址与演示假值 |
| [.github/workflows/ci.yml](../../.github/workflows/ci.yml) | 139 | +62 / −3 | 固定 Linux 全量检查、Compose、HTTP 和消息冒烟 |
| [AGENTS.md](../../AGENTS.md) | 113 | +5 / −3 | 保留八步流程并更新阶段 2 常用命令 |
| [README.md](../../README.md) | 234 | +83 / −18 | 运行、配置、测试、接口、压测与限制说明 |
| [cmd/README.md](../../cmd/README.md) | 13 | +4 / −1 | 分组目录的职责、边界与阅读入口 |
| [cmd/loadgen/main.go](../../cmd/loadgen/main.go) | 20 | +20 / −0 | 压测入口只传递参数、环境读取器与输出；调度和控制端口由 loadgen.Run 管理。 |
| [cmd/order-service/main.go](../../cmd/order-service/main.go) | 19 | +19 / −0 | 订单服务入口只转换退出码；资源装配、信号处理和请求排空由 orderservice.Run 完成。 |
| [cmd/order-worker/main.go](../../cmd/order-worker/main.go) | 19 | +19 / −0 | 订单消费者入口只负责退出码，连接和停机顺序由 orderworker.Run 管理。 |
| [deploy/docker-compose.yml](../../deploy/docker-compose.yml) | 225 | +125 / −7 | 七服务部署、资源上限、Redis 持久化与健康探测 |
| [deploy/mysql/init/003_order_sales_test_databases.sql](../../deploy/mysql/init/003_order_sales_test_databases.sql) | 8 | +8 / −0 | 订单与销量分别使用专用测试库，避免 go test 并发运行不同包时相互清理。 |
| [deploy/mysql/init/README.md](../../deploy/mysql/init/README.md) | 9 | +1 / −1 | 分组目录的职责、边界与阅读入口 |
| [docs/decisions.md](../../docs/decisions.md) | 119 | +27 / −0 | 设计决定 |
| [docs/learn/README.md](../../docs/learn/README.md) | 9 | +1 / −1 | 分组目录的职责、边界与阅读入口 |
| [docs/learn/phase-02-order.md](../../docs/learn/phase-02-order.md) | 525 | +525 / −0 | 阶段 2 学习材料：订单、缓存与消息 |
| [docs/notes.md](../../docs/notes.md) | 310 | +68 / −0 | 实测记录 |
| [docs/reports/README.md](../../docs/reports/README.md) | 9 | +1 / −1 | 分组目录的职责、边界与阅读入口 |
| [docs/reports/phase-02-order.md](../../docs/reports/phase-02-order.md) | 702 | +702 / −0 | 阶段 2 汇报书：订单服务、缓存与消息 |
| [docs/代码地图.md](../../docs/代码地图.md) | 191 | +75 / −6 | OpsDoctor 代码地图 |
| [go.mod](../../go.mod) | 59 | +16 / −1 | 固定新增客户端和治理库版本 |
| [go.sum](../../go.sum) | 151 | +54 / −6 | 依赖版本的内容校验和 |
| [internal/README.md](../../internal/README.md) | 12 | +1 / −0 | 分组目录的职责、边界与阅读入口 |
| [internal/doctor/auth/auth.go](../../internal/doctor/auth/auth.go) | 141 | +26 / −17 | 医生登录要用的令牌与密码规则。 |
| [internal/doctor/auth/auth_test.go](../../internal/doctor/auth/auth_test.go) | 131 | +7 / −2 | 认证规则的边界与伪造令牌测试。 |
| [internal/doctor/auth/doc.go](../../internal/doctor/auth/doc.go) | 11 | +1 / −1 | Package auth 医生认证的凭据规则，目前还没有接到 HTTP。 |
| [internal/loadgen/app.go](../../internal/loadgen/app.go) | 113 | +113 / −0 | 启动压测进程，管理控制端口、统计输出和退出。 |
| [internal/loadgen/client.go](../../internal/loadgen/client.go) | 93 | +93 / −0 | 发送真实 HTTP 请求，并按约定重试超时下单。 |
| [internal/loadgen/config.go](../../internal/loadgen/config.go) | 74 | +74 / −0 | 读取压测工具的目标地址、速率和运行时长。 |
| [internal/loadgen/control.go](../../internal/loadgen/control.go) | 95 | +95 / −0 | 通过本机控制接口调整速率和热点商品。 |
| [internal/loadgen/doc.go](../../internal/loadgen/doc.go) | 17 | +17 / −0 | Package loadgen 为靶场持续生成真实 HTTP 请求。 |
| [internal/loadgen/generator.go](../../internal/loadgen/generator.go) | 157 | +157 / −0 | 按指定速率选择请求类型和商品，并交给固定工作者。 |
| [internal/loadgen/loadgen_test.go](../../internal/loadgen/loadgen_test.go) | 200 | +200 / −0 | 验证压测请求分布、重试身份和真实统计口径。 |
| [internal/loadgen/stats.go](../../internal/loadgen/stats.go) | 133 | +133 / −0 | 从实际完成的 HTTP 尝试计算吞吐、错误率和延迟。 |
| [internal/platform/README.md](../../internal/platform/README.md) | 17 | +2 / −1 | 分组目录的职责、边界与阅读入口 |
| [internal/platform/adminserver/doc.go](../../internal/platform/adminserver/doc.go) | 11 | +1 / −1 | Package adminserver 提供独立管理端口的健康检查。 |
| [internal/platform/adminserver/server.go](../../internal/platform/adminserver/server.go) | 56 | +10 / −6 | 管理端口的健康检查和有界关闭。 |
| [internal/platform/adminserver/server_test.go](../../internal/platform/adminserver/server_test.go) | 51 | +4 / −1 | 在内存中验证健康状态和管理接口边界。 |
| [internal/platform/config/config.go](../../internal/platform/config/config.go) | 104 | +21 / −13 | 把环境变量转换为有范围约束的配置值。 |
| [internal/platform/config/config_test.go](../../internal/platform/config/config_test.go) | 81 | +4 / −1 | 用内存环境验证配置读取和错误提示。 |
| [internal/platform/config/doc.go](../../internal/platform/config/doc.go) | 11 | +1 / −1 | Package config 把环境配置转成启动前可以信任的值。 |
| [internal/platform/grpcx/doc.go](../../internal/platform/grpcx/doc.go) | 13 | +1 / −1 | Package grpcx 处理一元 RPC 的通用拦截和有界停机。 |
| [internal/platform/grpcx/interceptor.go](../../internal/platform/grpcx/interceptor.go) | 60 | +10 / −6 | 给每个一元 RPC 加日志和 panic 恢复。 |
| [internal/platform/grpcx/interceptor_test.go](../../internal/platform/grpcx/interceptor_test.go) | 67 | +4 / −1 | 验证拦截器顺序与日志脱敏。 |
| [internal/platform/grpcx/shutdown.go](../../internal/platform/grpcx/shutdown.go) | 37 | +6 / −3 | 给 gRPC 的优雅停机加上总预算。 |
| [internal/platform/grpcx/shutdown_test.go](../../internal/platform/grpcx/shutdown_test.go) | 78 | +6 / −2 | 证明 gRPC 停机超时能取消在途请求。 |
| [internal/platform/httpx/doc.go](../../internal/platform/httpx/doc.go) | 11 | +1 / −1 | Package httpx 集中处理 HTTP 入口的通用保护与统一响应。 |
| [internal/platform/httpx/httpx.go](../../internal/platform/httpx/httpx.go) | 156 | +29 / −19 | HTTP 请求进入业务前后的通用保护。 |
| [internal/platform/httpx/httpx_test.go](../../internal/platform/httpx/httpx_test.go) | 112 | +7 / −2 | 验证 HTTP 中间件组合后的实际响应。 |
| [internal/platform/lifecycle/doc.go](../../internal/platform/lifecycle/doc.go) | 11 | +1 / −1 | Package lifecycle 将退出信号转换成取消通知，再按顺序清理资源。 |
| [internal/platform/lifecycle/lifecycle.go](../../internal/platform/lifecycle/lifecycle.go) | 68 | +15 / −11 | 把退出信号和资源清理组织成可复用流程。 |
| [internal/platform/lifecycle/lifecycle_test.go](../../internal/platform/lifecycle/lifecycle_test.go) | 76 | +8 / −3 | 验证清理顺序、取消信号和停机预算。 |
| [internal/platform/logging/doc.go](../../internal/platform/logging/doc.go) | 11 | +1 / −1 | Package logging 把服务与请求关联信息加入 JSON 日志。 |
| [internal/platform/logging/logging.go](../../internal/platform/logging/logging.go) | 163 | +27 / −15 | 在 JSON 日志中自动关联请求与服务。 |
| [internal/platform/logging/logging_test.go](../../internal/platform/logging/logging_test.go) | 69 | +4 / −0 | 检查日志字段、UTC 时间和分组之后的关联信息。 |
| [internal/platform/migrate/doc.go](../../internal/platform/migrate/doc.go) | 14 | +1 / −1 | Package migrate 在同一数据库会话持锁执行嵌入式迁移。 |
| [internal/platform/migrate/files.go](../../internal/platform/migrate/files.go) | 69 | +6 / −3 | 读取并校验迁移文件，不执行 SQL。 |
| [internal/platform/migrate/files_test.go](../../internal/platform/migrate/files_test.go) | 65 | +6 / −2 | 验证迁移文件和历史校验规则。 |
| [internal/platform/migrate/migrate.go](../../internal/platform/migrate/migrate.go) | 238 | +36 / −27 | 在同一 MySQL 会话中串行执行迁移并记录历史。 |
| [internal/platform/migrate/migrate_integration_test.go](../../internal/platform/migrate/migrate_integration_test.go) | 128 | +14 / −9 | 在专用数据库验证迁移和命名锁。 |
| [internal/platform/mysqlx/doc.go](../../internal/platform/mysqlx/doc.go) | 12 | +1 / −1 | Package mysqlx 建立配置一致的 MySQL 连接池。 |
| [internal/platform/mysqlx/mysql.go](../../internal/platform/mysqlx/mysql.go) | 102 | +14 / −10 | 建立经过校验的 MySQL 连接池。 |
| [internal/platform/mysqlx/mysql_integration_test.go](../../internal/platform/mysqlx/mysql_integration_test.go) | 46 | +5 / −2 | 用真实 MySQL 检查连接池和会话时区。 |
| [internal/platform/mysqlx/mysql_test.go](../../internal/platform/mysqlx/mysql_test.go) | 45 | +5 / −2 | 离线验证 MySQL 配置规范化。 |
| [internal/platform/resilience/breaker.go](../../internal/platform/resilience/breaker.go) | 71 | +71 / −0 | 下游连续不健康时，暂停继续向它发送请求。 |
| [internal/platform/resilience/doc.go](../../internal/platform/resilience/doc.go) | 14 | +14 / −0 | Package resilience 给外部调用加上限流、重试和熔断保护。 |
| [internal/platform/resilience/http.go](../../internal/platform/resilience/http.go) | 49 | +49 / −0 | 在 HTTP 入口控制请求数量和处理时间。 |
| [internal/platform/resilience/resilience_test.go](../../internal/platform/resilience/resilience_test.go) | 134 | +134 / −0 | 验证入口限流、重试边界和熔断恢复。 |
| [internal/platform/resilience/retry.go](../../internal/platform/resilience/retry.go) | 87 | +87 / −0 | 给可以重复执行的调用加上有上限的重试。 |
| [internal/shop/README.md](../../internal/shop/README.md) | 17 | +8 / −3 | 分组目录的职责、边界与阅读入口 |
| [internal/shop/app/config.go](../../internal/shop/app/config.go) | 28 | +5 / −2 | 读取各 shop 程序共用的数据库配置。 |
| [internal/shop/app/config_test.go](../../internal/shop/app/config_test.go) | 40 | +4 / −1 | 验证连接池默认值和参数之间的约束。 |
| [internal/shop/app/doc.go](../../internal/shop/app/doc.go) | 12 | +2 / −1 | Package app 保存 shop 程序共同的数据库配置读取步骤。 |
| [internal/shop/app/inventoryservice/app.go](../../internal/shop/app/inventoryservice/app.go) | 108 | +14 / −9 | 库存进程的资源组装与退出流程。 |
| [internal/shop/app/inventoryservice/config.go](../../internal/shop/app/inventoryservice/config.go) | 39 | +6 / −3 | 库存进程自己的监听地址与启动关闭预算。 |
| [internal/shop/app/inventoryservice/doc.go](../../internal/shop/app/inventoryservice/doc.go) | 11 | +1 / −1 | Package inventoryservice 组装库存进程并拥有它创建的资源。 |
| [internal/shop/app/orderservice/app.go](../../internal/shop/app/orderservice/app.go) | 220 | +220 / −0 | 装配订单服务的依赖、路由和退出流程。 |
| [internal/shop/app/orderservice/config.go](../../internal/shop/app/orderservice/config.go) | 106 | +106 / −0 | 读取订单进程的配置，并检查各层时间预算。 |
| [internal/shop/app/orderservice/config_test.go](../../internal/shop/app/orderservice/config_test.go) | 48 | +48 / −0 | 验证订单服务的治理参数和时间预算关系。 |
| [internal/shop/app/orderservice/connection_test.go](../../internal/shop/app/orderservice/connection_test.go) | 53 | +53 / −0 | 验证订单服务在首次库存连接就绪后才开放入口。 |
| [internal/shop/app/orderservice/doc.go](../../internal/shop/app/orderservice/doc.go) | 15 | +15 / −0 | Package orderservice 组装订单 HTTP 进程。 |
| [internal/shop/app/orderservice/metrics.go](../../internal/shop/app/orderservice/metrics.go) | 42 | +42 / −0 | 记录下单进入“结果待核对”的告警与计数。 |
| [internal/shop/app/orderservice/metrics_test.go](../../internal/shop/app/orderservice/metrics_test.go) | 55 | +55 / −0 | 检查待核对日志与指标的实际内容。 |
| [internal/shop/app/orderworker/app.go](../../internal/shop/app/orderworker/app.go) | 125 | +125 / −0 | 启动订单消息消费者，并管理它的停机顺序。 |
| [internal/shop/app/orderworker/doc.go](../../internal/shop/app/orderworker/doc.go) | 10 | +10 / −0 | Package orderworker 组装订单消息消费者进程。 |
| [internal/shop/app/shopctl/app.go](../../internal/shop/app/shopctl/app.go) | 87 | +16 / −8 | 分派宿主机调试命令并组织造数流程。 |
| [internal/shop/app/shopctl/check.go](../../internal/shop/app/shopctl/check.go) | 61 | +61 / −0 | 提供订单件数与商品销量的只读核对命令。 |
| [internal/shop/app/shopctl/check_test.go](../../internal/shop/app/shopctl/check_test.go) | 39 | +39 / −0 | 验证核对命令不会把非零差值报告成成功。 |
| [internal/shop/app/shopctl/doc.go](../../internal/shop/app/shopctl/doc.go) | 14 | +3 / −1 | Package shopctl 组装宿主机造数与库存调试命令。 |
| [internal/shop/app/shopctl/stock_client.go](../../internal/shop/app/shopctl/stock_client.go) | 89 | +8 / −4 | 解析库存命令并通过正式 gRPC 契约调用服务。 |
| [internal/shop/app/shopctl/stock_client_test.go](../../internal/shop/app/shopctl/stock_client_test.go) | 40 | +4 / −1 | 验证命令行参数不会被误解或截断。 |
| [internal/shop/events/consumer.go](../../internal/shop/events/consumer.go) | 117 | +117 / −0 | 逐条处理 Kafka 消息，并在数据库成功后提交读取位置。 |
| [internal/shop/events/consumer_test.go](../../internal/shop/events/consumer_test.go) | 163 | +163 / −0 | 证明消费者在数据库成功后才提交位点。 |
| [internal/shop/events/doc.go](../../internal/shop/events/doc.go) | 15 | +15 / −0 | Package events 定义订单创建消息，并负责它在 Kafka 中的发送与接收。 |
| [internal/shop/events/model.go](../../internal/shop/events/model.go) | 36 | +36 / −0 | 定义订单创建事件的 JSON 契约。 |
| [internal/shop/events/publisher.go](../../internal/shop/events/publisher.go) | 65 | +65 / −0 | 把订单创建消息发送到 Kafka。 |
| [internal/shop/events/topic.go](../../internal/shop/events/topic.go) | 72 | +72 / −0 | 在接收业务流量之前准备订单消息主题。 |
| [internal/shop/inventory/doc.go](../../internal/shop/inventory/doc.go) | 17 | +1 / −1 | Package inventory 实现库存查询、扣减和归还，分开业务、数据与传输职责。 |
| [internal/shop/inventory/grpc_server.go](../../internal/shop/inventory/grpc_server.go) | 81 | +15 / −8 | 把 protobuf 请求接到库存业务并映射状态码。 |
| [internal/shop/inventory/grpc_server_test.go](../../internal/shop/inventory/grpc_server_test.go) | 103 | +9 / −4 | 验证 gRPC 错误契约和内存通道往返。 |
| [internal/shop/inventory/model.go](../../internal/shop/inventory/model.go) | 30 | +5 / −2 | 库存领域的快照类型与错误分类。 |
| [internal/shop/inventory/repository.go](../../internal/shop/inventory/repository.go) | 123 | +22 / −14 | 库存表的查询与原子更新。 |
| [internal/shop/inventory/repository_integration_test.go](../../internal/shop/inventory/repository_integration_test.go) | 170 | +13 / −6 | 真实 MySQL 的并发扣减与 deadline 实验。 |
| [internal/shop/inventory/repository_test.go](../../internal/shop/inventory/repository_test.go) | 48 | +4 / −1 | 离线区分暂时故障和 SQL 配置错误。 |
| [internal/shop/inventory/service.go](../../internal/shop/inventory/service.go) | 72 | +16 / −9 | 库存请求进入仓储前的业务规则。 |
| [internal/shop/inventory/service_test.go](../../internal/shop/inventory/service_test.go) | 108 | +11 / −3 | 离线验证参数规则、取消和仓储错误传递。 |
| [internal/shop/inventoryclient/doc.go](../../internal/shop/inventoryclient/doc.go) | 12 | +12 / −0 | Package inventoryclient 把商品和订单需要的库存操作转换成 gRPC 调用。 |
| [internal/shop/inventoryclient/inventory_client.go](../../internal/shop/inventoryclient/inventory_client.go) | 176 | +176 / −0 | 给库存 gRPC 调用加上时间限制和故障保护。 |
| [internal/shop/inventoryclient/inventory_client_test.go](../../internal/shop/inventoryclient/inventory_client_test.go) | 154 | +154 / −0 | 用假的 RPC 检查库存客户端的保护规则。 |
| [internal/shop/migrations/doc.go](../../internal/shop/migrations/doc.go) | 15 | +1 / −1 | Package migrations 保存 shop 五张表与嵌入入口。 |
| [internal/shop/migrations/migrations.go](../../internal/shop/migrations/migrations.go) | 28 | +6 / −3 | 把 shop 的 SQL 文件嵌入并交给通用迁移器。 |
| [internal/shop/order/cache.go](../../internal/shop/order/cache.go) | 159 | +159 / −0 | 用 Redis 保存幂等占位、创建结果和待核对状态。 |
| [internal/shop/order/cache_integration_test.go](../../internal/shop/order/cache_integration_test.go) | 170 | +170 / −0 | 用真实 Redis 验证幂等占位与原子更新。 |
| [internal/shop/order/doc.go](../../internal/shop/order/doc.go) | 20 | +20 / −0 | Package order 负责下单、订单查询和请求去重。 |
| [internal/shop/order/http_handler.go](../../internal/shop/order/http_handler.go) | 126 | +126 / −0 | 把订单 HTTP 请求转换成业务调用。 |
| [internal/shop/order/http_handler_test.go](../../internal/shop/order/http_handler_test.go) | 141 | +141 / −0 | 验证订单 HTTP 契约和严格输入解析。 |
| [internal/shop/order/idempotency.go](../../internal/shop/order/idempotency.go) | 57 | +57 / −0 | 描述一个幂等键从占位到完成或待核对的状态。 |
| [internal/shop/order/model.go](../../internal/shop/order/model.go) | 77 | +77 / −0 | 定义订单请求、订单结果和业务错误。 |
| [internal/shop/order/repository.go](../../internal/shop/order/repository.go) | 111 | +111 / −0 | 保存订单，并按主键或用户读取订单。 |
| [internal/shop/order/repository_integration_test.go](../../internal/shop/order/repository_integration_test.go) | 106 | +106 / −0 | 在真实 MySQL 中验证订单保存、详情和分页。 |
| [internal/shop/order/service.go](../../internal/shop/order/service.go) | 202 | +202 / −0 | 按顺序完成下单，并在失败时判断能否安全重试。 |
| [internal/shop/order/service_test.go](../../internal/shop/order/service_test.go) | 309 | +309 / −0 | 逐个模拟下单步骤失败，核对库存补偿与幂等记录。 |
| [internal/shop/product/cache.go](../../internal/shop/product/cache.go) | 96 | +96 / −0 | 保存商品缓存，并把过期时间打散。 |
| [internal/shop/product/cache_integration_test.go](../../internal/shop/product/cache_integration_test.go) | 73 | +73 / −0 | 在真实 Redis 中检查商品缓存的内容和过期规则。 |
| [internal/shop/product/cache_test.go](../../internal/shop/product/cache_test.go) | 58 | +58 / −0 | 检查商品缓存的内容和有效时间。 |
| [internal/shop/product/doc.go](../../internal/shop/product/doc.go) | 19 | +19 / −0 | Package product 负责商品价格查询和商品详情。 |
| [internal/shop/product/http_handler.go](../../internal/shop/product/http_handler.go) | 52 | +52 / −0 | 接收商品详情请求，把业务结果转换成统一 HTTP 响应。 |
| [internal/shop/product/http_handler_test.go](../../internal/shop/product/http_handler_test.go) | 62 | +62 / −0 | 检查商品接口的错误分类与库存展示。 |
| [internal/shop/product/model.go](../../internal/shop/product/model.go) | 44 | +44 / −0 | 定义商品查询使用的数据和错误。 |
| [internal/shop/product/repository.go](../../internal/shop/product/repository.go) | 42 | +42 / −0 | 按商品主键读取名称和价格。 |
| [internal/shop/product/service.go](../../internal/shop/product/service.go) | 116 | +116 / −0 | 串起商品缓存、数据库查询和库存展示。 |
| [internal/shop/product/service_test.go](../../internal/shop/product/service_test.go) | 216 | +216 / −0 | 验证商品查询的缓存顺序、并发回源和库存降级。 |
| [internal/shop/sales/doc.go](../../internal/shop/sales/doc.go) | 14 | +14 / −0 | Package sales 把订单消息累加成商品销量，并提供订单与销量的总量核对。 |
| [internal/shop/sales/model.go](../../internal/shop/sales/model.go) | 17 | +17 / −0 | 表示订单数量与已统计销量的核对结果。 |
| [internal/shop/sales/repository.go](../../internal/shop/sales/repository.go) | 85 | +85 / −0 | 在一个 MySQL 事务里完成订单去重和销量累加。 |
| [internal/shop/sales/repository_integration_test.go](../../internal/shop/sales/repository_integration_test.go) | 136 | +136 / −0 | 在真实 MySQL 中验证销量去重与事务回滚。 |
| [internal/shop/sales/service.go](../../internal/shop/sales/service.go) | 37 | +37 / −0 | 在处理订单消息之前验证业务字段。 |
| [internal/shop/seed/doc.go](../../internal/shop/seed/doc.go) | 13 | +1 / −1 | Package seed 建立靶场的商品、库存与历史订单基线。 |
| [internal/shop/seed/repository.go](../../internal/shop/seed/repository.go) | 119 | +16 / −10 | 批量造数的五表参数和事务写入。 |
| [internal/shop/seed/seed.go](../../internal/shop/seed/seed.go) | 69 | +9 / −6 | 控制完整规模与 small 模式的造数进度。 |
| [internal/shop/seed/seed_integration_test.go](../../internal/shop/seed/seed_integration_test.go) | 109 | +8 / −3 | 用独立 MySQL 库验证续跑、跳过与回滚。 |
| [internal/shop/seed/seed_test.go](../../internal/shop/seed/seed_test.go) | 37 | +6 / −3 | 在内存中核对造数行数、编号和占位符。 |
| [tools/archcheck/check.go](../../tools/archcheck/check.go) | 402 | +59 / −44 | 按目录、导入和文件访问规则检查架构边界。 |
| [tools/archcheck/check_test.go](../../tools/archcheck/check_test.go) | 149 | +9 / −3 | 用故意违规的假模块检验架构规则。 |
| [tools/archcheck/doc.go](../../tools/archcheck/doc.go) | 12 | +1 / −1 | Package main 把目录分层和领域隔离变成可以在 CI 执行的检查。 |
| [tools/archcheck/main.go](../../tools/archcheck/main.go) | 40 | +5 / −2 | 把架构检查接到命令行与 CI。 |
| [tools/commentcheck/check.go](../../tools/commentcheck/check.go) | 283 | +37 / −26 | 扫描 Go 声明并检查代码层注释。 |
| [tools/commentcheck/check_test.go](../../tools/commentcheck/check_test.go) | 166 | +15 / −6 | 验证代码层规则与薄入口的例外。 |
| [tools/commentcheck/doc.go](../../tools/commentcheck/doc.go) | 14 | +1 / −1 | Package main 把目录、文件、代码三层说明的可检查部分接到 CI。 |
| [tools/commentcheck/files.go](../../tools/commentcheck/files.go) | 154 | +18 / −10 | 检查文件说明与包内文件清单，并输出代码地图。 |
| [tools/commentcheck/files_test.go](../../tools/commentcheck/files_test.go) | 109 | +6 / −1 | 证明三层说明的机器规则能发现缺失，并生成可点击的文件地图。 |
| [tools/commentcheck/main.go](../../tools/commentcheck/main.go) | 70 | +7 / −3 | 分派注释检查、符号清单和文件地图三种命令模式。 |
| [进度.md](../../进度.md) | 163 | +15 / −15 | 交付与工程状态，学习验收保留未勾选 |
| [项目计划书.md](../../项目计划书.md) | 1859 | +23 / −3 | 用户批准的失败边界、错误码与配置契约 |
<!-- FILES_END -->

### 3.3 类型与函数清单

依据 commentcheck -list 汇总新增文件及新增核心函数，再补调用方。行号在最终收尾时重新生成。

<!-- SYMBOLS_START -->
#### cmd/loadgen

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| main | [cmd/loadgen/main.go:15](../../cmd/loadgen/main.go#L15) | main 等待在途请求排空与最终报告输出后再决定退出码 | Go 运行时 |

#### cmd/order-service

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| main | [cmd/order-service/main.go:14](../../cmd/order-service/main.go#L14) | main 等 Run 清理资源后再退出，避免 os.Exit 绕过仍未执行的关闭动作 | Go 运行时 |

#### cmd/order-worker

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| main | [cmd/order-worker/main.go:14](../../cmd/order-worker/main.go#L14) | main 在 Run 返回并完成清理后才调用 os.Exit，不提前跳过数据库与 Kafka 的关闭 | Go 运行时 |

#### internal/loadgen

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| logReport | [internal/loadgen/app.go:111](../../internal/loadgen/app.go#L111) | logReport 把统计作为独立 JSON 对象输出，脚本可以读取数值而不必解析一段中文句子 | Run、printWindows |
| Run | [internal/loadgen/app.go:39](../../internal/loadgen/app.go#L39) | Run 把启动准备排除在统计之外，实际完成数从工作者开始运行时累计 | cmd/loadgen.main |
| printWindows | [internal/loadgen/app.go:97](../../internal/loadgen/app.go#L97) | printWindows 每十秒重置一次窗口，总计保存在另一份直方图中，不受这里影响 | Run 的报告 goroutine |
| *generator.execute | [internal/loadgen/client.go:28](../../internal/loadgen/client.go#L28) | execute 把一次逻辑下单限制在两次 HTTP 尝试，第二次沿用同一个 job 的 key | generator.run 的工作者 |
| *generator.attempt | [internal/loadgen/client.go:40](../../internal/loadgen/client.go#L40) | attempt 只接受 HTTP 2xx 且 code 为 OK 的完整 JSON，避免代理返回 HTML 200 也被计成成功 | generator.execute |
| timeoutError | [internal/loadgen/client.go:87](../../internal/loadgen/client.go#L87) | timeoutError 不把所有网络故障都当成超时，连接拒绝和普通业务错误不在重试范围内 | generator.attempt |
| Config | [internal/loadgen/config.go:26](../../internal/loadgen/config.go#L26) | Config 保存已经检查过的参数 | loadConfig 返回，Run 和 generator 使用 |
| loadConfig | [internal/loadgen/config.go:37](../../internal/loadgen/config.go#L37) | loadConfig 把环境作为默认值，命令行覆盖只影响本次进程，不写回 .env | Run、配置测试 |
| controlSettings | [internal/loadgen/control.go:27](../../internal/loadgen/control.go#L27) | controlSettings 是一份可复制的配置快照，JSON 中的名字也是控制接口契约 | Run、controlState、控制接口 |
| controlState | [internal/loadgen/control.go:33](../../internal/loadgen/control.go#L33) | controlState 用互斥锁保护两个字段一起更新，调度者不会看到半次更新 | Run 构造；调度和控制接口共享 |
| *controlState.snapshot | [internal/loadgen/control.go:40](../../internal/loadgen/control.go#L40) | snapshot 返回值副本，调用方离开锁后不能改写共享配置 | generator.schedule、controlRouter |
| *controlState.update | [internal/loadgen/control.go:47](../../internal/loadgen/control.go#L47) | update 使用指针区分“字段省略”与“明确设为零”，后者在这里是有效操作 | controlRouter 的 POST 回调 |
| controlRouter | [internal/loadgen/control.go:70](../../internal/loadgen/control.go#L70) | controlRouter 只在专用管理端口安装，Compose 把它映射到宿主机回环地址 | Run |
| *generator.schedule | [internal/loadgen/generator.go:125](../../internal/loadgen/generator.go#L125) | schedule 每五毫秒积累“应发送多少次”的额度，实际完成 QPS 由 statistics 独立计算 | generator.run |
| requestKind | [internal/loadgen/generator.go:28](../../internal/loadgen/generator.go#L28) | requestKind 的连续编号也用于统计数组下标，不接受外部输入直接设置 | job、统计类型下标 |
| job | [internal/loadgen/generator.go:47](../../internal/loadgen/generator.go#L47) | job 是一次逻辑操作 | nextJob 生成；schedule 和 execute 传递 |
| generator | [internal/loadgen/generator.go:55](../../internal/loadgen/generator.go#L55) | generator 把调度数据和发送依赖放在一起，随机源只允许 schedule 所在 goroutine 使用 | Run 构造；调度和工作者使用 |
| newGenerator | [internal/loadgen/generator.go:69](../../internal/loadgen/generator.go#L69) | newGenerator 用 v=100 分散最热的一小组商品，仍让它们远比长尾商品常见 | Run、测试构造 |
| *generator.nextJob | [internal/loadgen/generator.go:76](../../internal/loadgen/generator.go#L76) | nextJob 用一次百分位抽签选择接口，因此长期接近 70/20/10，不保证每十个请求都恰好凑齐 | generator.schedule |
| *generator.run | [internal/loadgen/generator.go:95](../../internal/loadgen/generator.go#L95) | run 停止时先停调度，再让正在发送的请求结束 | Run 的调度 goroutine |
| TestStatistics | [internal/loadgen/loadgen_test.go:131](../../internal/loadgen/loadgen_test.go#L131) | TestStatistics 用已知的一百个样本核对百分位，不把配置 QPS 直接写进报告 | go test |
| TestControl | [internal/loadgen/loadgen_test.go:155](../../internal/loadgen/loadgen_test.go#L155) | TestControl 使用内存 HTTP 请求检查部分更新，非法值不能改动现有设置 | go test |
| TestBoundedWorkers | [internal/loadgen/loadgen_test.go:179](../../internal/loadgen/loadgen_test.go#L179) | TestBoundedWorkers 让发送持续等到请求超时，确认高 QPS 也不会无限启动工作者 | go test |
| roundTripFunc | [internal/loadgen/loadgen_test.go:31](../../internal/loadgen/loadgen_test.go#L31) | roundTripFunc 让测试只提供一个函数，不必创建监听端口才能观察请求 | 本包测试函数及其注入的业务接口 |
| roundTripFunc.RoundTrip | [internal/loadgen/loadgen_test.go:34](../../internal/loadgen/loadgen_test.go#L34) | RoundTrip 满足 http.RoundTripper 接口，http.Client 会照常执行请求流程 | 本包测试函数及其注入的业务接口 |
| testGenerator | [internal/loadgen/loadgen_test.go:39](../../internal/loadgen/loadgen_test.go#L39) | testGenerator 使用不会联网的客户端 | 本包测试函数及其注入的业务接口 |
| TestTrafficDistribution | [internal/loadgen/loadgen_test.go:47](../../internal/loadgen/loadgen_test.go#L47) | TestTrafficDistribution 不要求每十次恰好七二一，而是核对大量固定随机样本的比例 | go test |
| TestRetryKeepsKey | [internal/loadgen/loadgen_test.go:83](../../internal/loadgen/loadgen_test.go#L83) | TestRetryKeepsKey 检查超时重试与不该重试的响应，同时验证统计保留每次尝试 | go test |
| percentile | [internal/loadgen/stats.go:120](../../internal/loadgen/stats.go#L120) | percentile 使用向上取整的名次，再累加桶计数，找到包含该名次的桶 | statistics.snapshot |
| histogram | [internal/loadgen/stats.go:26](../../internal/loadgen/stats.go#L26) | histogram 使用固定数组，持续压测不会无限保存每条请求的延迟样本 | statistics 内部累计 |
| statistics | [internal/loadgen/stats.go:36](../../internal/loadgen/stats.go#L36) | statistics 的一把锁保护两个直方图，窗口和总计看到同一次记录 | generator、控制与报告循环 |
| report | [internal/loadgen/stats.go:45](../../internal/loadgen/stats.go#L45) | report 是日志与控制接口使用的数值快照，不含内部锁或大数组 | snapshot 返回；日志与 GET /control 输出 |
| newStatistics | [internal/loadgen/stats.go:61](../../internal/loadgen/stats.go#L61) | newStatistics 在开始调度前创建，不把配置和连接准备时间算进压测速率 | newGenerator |
| *statistics.record | [internal/loadgen/stats.go:64](../../internal/loadgen/stats.go#L64) | record 向上取整延迟，百分位不会因取整而比该桶内的真实请求更快 | generator.execute |
| *statistics.drop | [internal/loadgen/stats.go:88](../../internal/loadgen/stats.go#L88) | drop 记录没有真正发出的计划请求，既不冒充完成数，也不把它隐去 | generator.schedule |
| *statistics.snapshot | [internal/loadgen/stats.go:96](../../internal/loadgen/stats.go#L96) | snapshot 在持锁时复制小报表 | printWindows、Run、controlRouter |

#### internal/platform/resilience

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| BreakerConfig | [internal/platform/resilience/breaker.go:26](../../internal/platform/resilience/breaker.go#L26) | BreakerConfig 描述何时暂停请求，以及多久后允许少量请求探路 | orderservice.loadConfig、inventoryclient.New |
| Breaker | [internal/platform/resilience/breaker.go:36](../../internal/platform/resilience/breaker.go#L36) | Breaker 隐藏第三方的结果泛型 | inventoryclient.Client |
| NewBreaker | [internal/platform/resilience/breaker.go:42](../../internal/platform/resilience/breaker.go#L42) | NewBreaker 由调用方说明哪些错误算下游故障，避免库存不足也把服务“熔断” | inventoryclient.New |
| *Breaker.Execute | [internal/platform/resilience/breaker.go:60](../../internal/platform/resilience/breaker.go#L60) | Execute 在关闭或半开时调用 operation | inventoryclient.GetStock、Deduct |
| *Breaker.State | [internal/platform/resilience/breaker.go:71](../../internal/platform/resilience/breaker.go#L71) | State 读取底层状态，测试用它检查恢复过程，而不是猜测内部计数 | 熔断恢复测试 |
| RateLimit | [internal/platform/resilience/http.go:26](../../internal/platform/resilience/http.go#L26) | RateLimit 为整个服务创建一个桶 | orderservice.routes |
| RequestBudget | [internal/platform/resilience/http.go:42](../../internal/platform/resilience/http.go#L42) | RequestBudget 把总预算放进请求 context | orderservice.routes |
| TestBreakerRecovery | [internal/platform/resilience/resilience_test.go:109](../../internal/platform/resilience/resilience_test.go#L109) | TestBreakerRecovery 用短窗口观察完整状态变化，确认打开期间没有偷偷执行下游回调 | go test |
| TestRateLimit | [internal/platform/resilience/resilience_test.go:30](../../internal/platform/resilience/resilience_test.go#L30) | TestRateLimit 用一个几乎不会在测试期间补满的桶，明确看到“放行一次，再拒绝一次” | go test |
| TestRetryPolicy | [internal/platform/resilience/resilience_test.go:52](../../internal/platform/resilience/resilience_test.go#L52) | TestRetryPolicy 固定随机增量，让指数退避的每一步都可以直接核对 | go test |
| TestRetryCancellation | [internal/platform/resilience/resilience_test.go:94](../../internal/platform/resilience/resilience_test.go#L94) | TestRetryCancellation 让第一次失败同时取消上下文，等待器必须立即结束 | go test |
| RetryPolicy | [internal/platform/resilience/retry.go:24](../../internal/platform/resilience/retry.go#L24) | RetryPolicy 把重试限制写在一起，避免每个调用各自决定什么时候再试 | orderservice.loadConfig、inventoryclient.Config |
| Retrier | [internal/platform/resilience/retry.go:31](../../internal/platform/resilience/retry.go#L31) | Retrier 保存固定策略 | inventoryclient.Client |
| NewRetrier | [internal/platform/resilience/retry.go:41](../../internal/platform/resilience/retry.go#L41) | NewRetrier 在启动时拒绝不合理的策略，不把配置问题留到第一次请求 | inventoryclient.New |
| *Retrier.Do | [internal/platform/resilience/retry.go:55](../../internal/platform/resilience/retry.go#L55) | Do 只根据调用方提供的 retryable 判断重试资格 | inventoryclient.GetStock |
| waitContext | [internal/platform/resilience/retry.go:78](../../internal/platform/resilience/retry.go#L78) | waitContext 不使用 Sleep，因为调用方取消时，不应该继续睡到退避时间结束 | Retrier.Do 经 wait 回调 |

#### internal/shop/app/orderservice

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| connectInventory | [internal/shop/app/orderservice/app.go:146](../../internal/shop/app/orderservice/app.go#L146) | connectInventory 把首次 DNS 查询和握手放进启动预算，避免第一位用户替服务承担建连等待 | Run、连接测试 |
| routes | [internal/shop/app/orderservice/app.go:164](../../internal/shop/app/orderservice/app.go#L164) | routes 先安装请求标识和恢复，再限流、限制正文与时间，最后才进入业务 | Run |
| serve | [internal/shop/app/orderservice/app.go:181](../../internal/shop/app/orderservice/app.go#L181) | serve 等两个端口都绑定成功才开始接流量，避免只启动了半个进程 | Run |
| normalHTTPError | [internal/shop/app/orderservice/app.go:215](../../internal/shop/app/orderservice/app.go#L215) | normalHTTPError 只忽略标准的正常关闭错误，绑定后的监听故障仍交给入口返回失败码 | serve |
| Run | [internal/shop/app/orderservice/app.go:61](../../internal/shop/app/orderservice/app.go#L61) | Run 的 result 是具名返回值，defer 可以把关闭连接的错误一并交给入口 | cmd/order-service.main |
| Config | [internal/shop/app/orderservice/config.go:29](../../internal/shop/app/orderservice/config.go#L29) | Config 中的 Database 含密码，禁止把整个结构体写进日志 | loadConfig 返回；Run 和 routes 使用 |
| loadConfig | [internal/shop/app/orderservice/config.go:47](../../internal/shop/app/orderservice/config.go#L47) | loadConfig 用局部读取函数收集错误，用户一次就能看到多个配置问题 | Run、配置测试 |
| kafkaBrokers | [internal/shop/app/orderservice/config.go:96](../../internal/shop/app/orderservice/config.go#L96) | kafkaBrokers 要求每项都包含主机和端口，空列表项通常是配置多写了一个逗号 | loadConfig |
| TestConfigBudgets | [internal/shop/app/orderservice/config_test.go:19](../../internal/shop/app/orderservice/config_test.go#L19) | TestConfigBudgets 每次只覆盖一个变量，便于知道是哪条配置规则拒绝了启动 | go test |
| TestConnectInventory | [internal/shop/app/orderservice/connection_test.go:26](../../internal/shop/app/orderservice/connection_test.go#L26) | TestConnectInventory 对照真实内存握手与持续拒绝拨号，启动不能把未就绪当成成功 | go test |
| reviewRecorder | [internal/shop/app/orderservice/metrics.go:22](../../internal/shop/app/orderservice/metrics.go#L22) | reviewRecorder 把日志与计数放在同一个动作里，避免某个失败分支只做其中一项 | newReviewRecorder 构造；order.Service 使用 |
| newReviewRecorder | [internal/shop/app/orderservice/metrics.go:28](../../internal/shop/app/orderservice/metrics.go#L28) | newReviewRecorder 使用 app 自己的注册表，不污染全局注册表，也便于测试重复创建服务 | Run、指标测试 |
| *reviewRecorder.RecordReview | [internal/shop/app/orderservice/metrics.go:39](../../internal/shop/app/orderservice/metrics.go#L39) | RecordReview 使用固定原因分类，不把原始连接错误、DSN 或请求体写入告警 | order.Service.unknown、Create 的遗留占位分支 |
| TestReviewRecorder | [internal/shop/app/orderservice/metrics_test.go:22](../../internal/shop/app/orderservice/metrics_test.go#L22) | TestReviewRecorder 验证调用一次便同时产生告警和计数，避免接口接上了却没有实际记录 | go test |

#### internal/shop/app/orderworker

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| Run | [internal/shop/app/orderworker/app.go:42](../../internal/shop/app/orderworker/app.go#L42) | Run 先停消费者，再关闭 Kafka 和数据库，不能在当前事务尚未完成时拆掉连接 | cmd/order-worker.main |
| serve | [internal/shop/app/orderworker/app.go:90](../../internal/shop/app/orderworker/app.go#L90) | serve 在管理端口可用后启动消费，任意循环失败都会取消另一侧并读齐退出结果 | Run |

#### internal/shop/app/shopctl

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| runCheck | [internal/shop/app/shopctl/check.go:28](../../internal/shop/app/shopctl/check.go#L28) | runCheck 不执行迁移，保持核对命令只读 | shopctl.Run 的 check 分支 |
| writeCheck | [internal/shop/app/shopctl/check.go:52](../../internal/shop/app/shopctl/check.go#L52) | writeCheck 只报告事实 | runCheck、输出测试 |
| TestCheckOutput | [internal/shop/app/shopctl/check_test.go:21](../../internal/shop/app/shopctl/check_test.go#L21) | TestCheckOutput 使用明确的总量，让输出必须来自传入结果，而不是固定写零 | go test |

#### internal/shop/events

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| MessageReader | [internal/shop/events/consumer.go:32](../../internal/shop/events/consumer.go#L32) | MessageReader 是消费者需要的最小能力，真实实现是 kafka.Reader | Consumer；kafka.Reader 与测试读取器实现 |
| Consumer | [internal/shop/events/consumer.go:44](../../internal/shop/events/consumer.go#L44) | Consumer 不拥有 reader 的关闭权，app 等 Run 返回后统一关闭连接 | orderworker.serve |
| NewReader | [internal/shop/events/consumer.go:52](../../internal/shop/events/consumer.go#L52) | NewReader 从消费组已提交的位置继续读取，新组则从最早保留的消息开始 | orderworker.Run |
| NewConsumer | [internal/shop/events/consumer.go:59](../../internal/shop/events/consumer.go#L59) | NewConsumer 要求有限的处理预算，停止服务时不能无限等一条消息 | orderworker.Run、消费测试 |
| *Consumer.Run | [internal/shop/events/consumer.go:67](../../internal/shop/events/consumer.go#L67) | Run 串行处理消息 | orderworker.serve |
| *Consumer.process | [internal/shop/events/consumer.go:93](../../internal/shop/events/consumer.go#L93) | process 把解析、处理、提交写在同一处，读者可以直接核对先后顺序 | Consumer.Run |
| TestConsumerFinishesCurrentMessage | [internal/shop/events/consumer_test.go:120](../../internal/shop/events/consumer_test.go#L120) | TestConsumerFinishesCurrentMessage 在处理数据库时取消服务，当前消息的独立预算仍然有效 | go test |
| TestRejectInvalidMessage | [internal/shop/events/consumer_test.go:143](../../internal/shop/events/consumer_test.go#L143) | TestRejectInvalidMessage 坏消息不提交，避免静默丢弃后看起来消费一切正常 | go test |
| fakeReader | [internal/shop/events/consumer_test.go:29](../../internal/shop/events/consumer_test.go#L29) | fakeReader 在同一个消费 goroutine 中使用，不需要为这些计数器加锁 | 本包测试函数及其注入的业务接口 |
| *fakeReader.FetchMessage | [internal/shop/events/consumer_test.go:38](../../internal/shop/events/consumer_test.go#L38) | FetchMessage 队列为空时等待取消，模拟消费者平时没有新消息的状态 | 本包测试函数及其注入的业务接口 |
| *fakeReader.CommitMessages | [internal/shop/events/consumer_test.go:49](../../internal/shop/events/consumer_test.go#L49) | CommitMessages 先触发顺序断言，再记录成功，提交失败时不增加已提交数 | 本包测试函数及其注入的业务接口 |
| testMessage | [internal/shop/events/consumer_test.go:61](../../internal/shop/events/consumer_test.go#L61) | testMessage 用真实 JSON 编码生成消息，避免手写字段与契约不一致 | 本包测试函数及其注入的业务接口 |
| testConsumer | [internal/shop/events/consumer_test.go:72](../../internal/shop/events/consumer_test.go#L72) | testConsumer 把日志送到 Discard，让预期失败不会混进测试输出 | 本包测试函数及其注入的业务接口 |
| TestConsumerCommitOrder | [internal/shop/events/consumer_test.go:82](../../internal/shop/events/consumer_test.go#L82) | TestConsumerCommitOrder 分别模拟数据库失败、位点提交失败和重复消息 | go test |
| OrderCreated | [internal/shop/events/model.go:20](../../internal/shop/events/model.go#L20) | OrderCreated 对应 order.created 的消息体，去重依据是 OrderID | order.Service.Create、sales.Service.Handle |
| OrderCreated.Validate | [internal/shop/events/model.go:30](../../internal/shop/events/model.go#L30) | Validate 在写入销量之前挡住损坏消息，避免错误数据污染汇总表 | Publisher.Publish、Consumer.process、sales.Service.Handle |
| Publisher | [internal/shop/events/publisher.go:25](../../internal/shop/events/publisher.go#L25) | Publisher 使用 kafka.Writer 自带的连接复用，不为每个订单重新连接 broker | orderservice.Run 创建；order.Service 借用 |
| NewPublisher | [internal/shop/events/publisher.go:39](../../internal/shop/events/publisher.go#L39) | NewPublisher 禁用自动建主题 | orderservice.Run |
| *Publisher.Publish | [internal/shop/events/publisher.go:48](../../internal/shop/events/publisher.go#L48) | Publish 收到错误时不声称发送成功 | order.Service.Create |
| *Publisher.Close | [internal/shop/events/publisher.go:61](../../internal/shop/events/publisher.go#L61) | Close 等发送中的调用结束后再执行，避免关闭过程与新的 Publish 争抢资源 | orderservice.Run 的 defer |
| EnsureTopic | [internal/shop/events/topic.go:33](../../internal/shop/events/topic.go#L33) | EnsureTopic 只创建计划书约定的主题，不修改已有主题的分区或删除数据 | orderservice.Run、orderworker.Run |

#### internal/shop/inventoryclient

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| *Client.Deduct | [internal/shop/inventoryclient/inventory_client.go:106](../../internal/shop/inventoryclient/inventory_client.go#L106) | Deduct 一次只发一条 RPC | order.Service.Create |
| *Client.Release | [internal/shop/inventoryclient/inventory_client.go:126](../../internal/shop/inventoryclient/inventory_client.go#L126) | Release 不受扣减熔断器阻挡，否则已经占用的库存可能连归还机会都没有 | order.Service.Create 的补偿分支 |
| healthyResult | [internal/shop/inventoryclient/inventory_client.go:140](../../internal/shop/inventoryclient/inventory_client.go#L140) | healthyResult 把已明确答复的业务拒绝算作服务正常，只有技术故障才增加失败数 | NewBreaker 回调、Deduct、Release |
| canceledResult | [internal/shop/inventoryclient/inventory_client.go:150](../../internal/shop/inventoryclient/inventory_client.go#L150) | canceledResult 排除调用方主动放弃的请求，它不能说明下游是否健康 | NewBreaker 的排除回调 |
| convertError | [internal/shop/inventoryclient/inventory_client.go:155](../../internal/shop/inventoryclient/inventory_client.go#L155) | convertError 不把 gRPC 类型泄露到业务分支中，HTTP 层只需判断领域错误 | GetStock、Deduct、Release |
| Config | [internal/shop/inventoryclient/inventory_client.go:33](../../internal/shop/inventoryclient/inventory_client.go#L33) | Config 由 app 从环境变量读取 | orderservice.loadConfig 返回；New 检查 |
| Client | [internal/shop/inventoryclient/inventory_client.go:40](../../internal/shop/inventoryclient/inventory_client.go#L40) | Client 共用一条 gRPC 连接 | orderservice.Run 注入 product 与 order |
| WriteError | [internal/shop/inventoryclient/inventory_client.go:49](../../internal/shop/inventoryclient/inventory_client.go#L49) | WriteError 表示写请求的错误 | Deduct、Release；order.outcomeUnknown 读取 |
| *WriteError.Error | [internal/shop/inventoryclient/inventory_client.go:55](../../internal/shop/inventoryclient/inventory_client.go#L55) | Error 提供可读的错误文本，底层原因仍通过 Unwrap 保留 | 标准 error 接口 |
| *WriteError.Unwrap | [internal/shop/inventoryclient/inventory_client.go:58](../../internal/shop/inventoryclient/inventory_client.go#L58) | Unwrap 让 errors.Is 和 errors.As 可以继续查看包裹的原因 | errors.Is、errors.As |
| *WriteError.OutcomeUnknown | [internal/shop/inventoryclient/inventory_client.go:61](../../internal/shop/inventoryclient/inventory_client.go#L61) | OutcomeUnknown 让业务层通过一个小接口识别结果不确定，不必导入本客户端的具体错误类型 | order.outcomeUnknown |
| New | [internal/shop/inventoryclient/inventory_client.go:64](../../internal/shop/inventoryclient/inventory_client.go#L64) | New 提前检查配置，避免服务已经接流量后才发现保护参数不合法 | orderservice.Run |
| *Client.GetStock | [internal/shop/inventoryclient/inventory_client.go:85](../../internal/shop/inventoryclient/inventory_client.go#L85) | GetStock 的所有尝试共享一个截止时间，不能每次重试都重新获得完整预算 | product.Service.Detail |
| TestWriteOnceAndBreaker | [internal/shop/inventoryclient/inventory_client_test.go:109](../../internal/shop/inventoryclient/inventory_client_test.go#L109) | TestWriteOnceAndBreaker 业务拒绝不能熔断 | go test |
| TestCanceledRequest | [internal/shop/inventoryclient/inventory_client_test.go:144](../../internal/shop/inventoryclient/inventory_client_test.go#L144) | TestCanceledRequest 确认请求还没发出时就取消，不会留下“可能已经扣减”的标记 | go test |
| fakeRPC | [internal/shop/inventoryclient/inventory_client_test.go:32](../../internal/shop/inventoryclient/inventory_client_test.go#L32) | fakeRPC 按调用次数返回结果 | 本包测试函数及其注入的业务接口 |
| *fakeRPC.GetStock | [internal/shop/inventoryclient/inventory_client_test.go:42](../../internal/shop/inventoryclient/inventory_client_test.go#L42) | GetStock 模拟先失败再恢复，同时记录传进来的预算 | 本包测试函数及其注入的业务接口 |
| *fakeRPC.Deduct | [internal/shop/inventoryclient/inventory_client_test.go:53](../../internal/shop/inventoryclient/inventory_client_test.go#L53) | Deduct 只记录写请求，不模拟真实库存，避免测试把关注点变成数据库逻辑 | 本包测试函数及其注入的业务接口 |
| *fakeRPC.Release | [internal/shop/inventoryclient/inventory_client_test.go:59](../../internal/shop/inventoryclient/inventory_client_test.go#L59) | Release 与 Deduct 分开计数，检查扣减熔断后仍然可以尝试归还 | 本包测试函数及其注入的业务接口 |
| testClient | [internal/shop/inventoryclient/inventory_client_test.go:65](../../internal/shop/inventoryclient/inventory_client_test.go#L65) | testClient 让熔断样本足够大，普通查询重试测试不会提前打开熔断器 | 本包测试函数及其注入的业务接口 |
| TestReadRetryAndDeadline | [internal/shop/inventoryclient/inventory_client_test.go:78](../../internal/shop/inventoryclient/inventory_client_test.go#L78) | TestReadRetryAndDeadline 同时核对成功恢复和达到次数上限，防止只测到成功路径 | go test |

#### internal/shop/order

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| *RedisIdempotency.Complete | [internal/shop/order/cache.go:120](../../internal/shop/order/cache.go#L120) | Complete 只把自己的处理中记录替换成完成结果，已经转成待核对的记录不能偷偷改回成功 | Service.Create |
| *RedisIdempotency.Review | [internal/shop/order/cache.go:127](../../internal/shop/order/cache.go#L127) | Review 保留原有占位期限，避免不确定请求在窗口内重新执行业务 | Service.unknown |
| *RedisIdempotency.replace | [internal/shop/order/cache.go:134](../../internal/shop/order/cache.go#L134) | replace 不把订单编号交给 Lua 的浮点数运算，完整 JSON 由 Go 编码后原样替换 | existing、Complete、Review |
| *RedisIdempotency.Abandon | [internal/shop/order/cache.go:150](../../internal/shop/order/cache.go#L150) | Abandon 只用于确定没有留下业务变更的路径 | Service.safeFailure |
| RedisIdempotency | [internal/shop/order/cache.go:38](../../internal/shop/order/cache.go#L38) | RedisIdempotency 保存固定策略 | orderservice.Run 创建；Service 借用 |
| NewRedisIdempotency | [internal/shop/order/cache.go:45](../../internal/shop/order/cache.go#L45) | NewRedisIdempotency 要求记录寿命长于处理预算，避免请求尚未结束 key 就消失 | orderservice.Run、Redis 集成测试 |
| requestFingerprint | [internal/shop/order/cache.go:53](../../internal/shop/order/cache.go#L53) | requestFingerprint 只比较三个已解析的整数，JSON 空格或字段顺序不会改变同一请求的身份 | RedisIdempotency.Reserve |
| *RedisIdempotency.Reserve | [internal/shop/order/cache.go:60](../../internal/shop/order/cache.go#L60) | Reserve 只有 SET NX 成功才授予执行权 | Service.Create |
| *RedisIdempotency.existing | [internal/shop/order/cache.go:80](../../internal/shop/order/cache.go#L80) | existing 在总处理预算已过时把遗留占位转为待核对，覆盖进程退出或 Redis 写状态失败的情况 | Reserve |
| TestIntegrationReservationOwnership | [internal/shop/order/cache_integration_test.go:115](../../internal/shop/order/cache_integration_test.go#L115) | TestIntegrationReservationOwnership 删除安全失败的占位后，新请求得到新的所有者，旧请求不能再修改它 | go test |
| TestIntegrationExpiredProcessing | [internal/shop/order/cache_integration_test.go:146](../../internal/shop/order/cache_integration_test.go#L146) | TestIntegrationExpiredProcessing 模拟进程退出留下的占位，不用真实等待十秒 | go test |
| integrationStore | [internal/shop/order/cache_integration_test.go:28](../../internal/shop/order/cache_integration_test.go#L28) | integrationStore 只有设置 TEST_REDIS_ADDR 才连接，地址不打印，测试键也不会写到业务数据库 0 | 本包测试函数及其注入的业务接口 |
| reserveResult | [internal/shop/order/cache_integration_test.go:58](../../internal/shop/order/cache_integration_test.go#L58) | reserveResult 把并发占位的三个返回值一起送回主测试，不在 goroutine 中调用 Fatal | 本包测试函数及其注入的业务接口 |
| TestIntegrationReservationCompetition | [internal/shop/order/cache_integration_test.go:65](../../internal/shop/order/cache_integration_test.go#L65) | TestIntegrationReservationCompetition 验证 SET NX 的竞争结果，再检查写回结果是否保留原 TTL | go test |
| respondError | [internal/shop/order/http_handler.go:104](../../internal/shop/order/http_handler.go#L104) | respondError 不向客户端暴露内部错误文本 | Handler 的三个入口 |
| HTTPService | [internal/shop/order/http_handler.go:29](../../internal/shop/order/http_handler.go#L29) | HTTPService 让 handler 测试可以只检查传输契约，不必顺便运行整个下单流程 | Handler；Service 与 HTTP 测试替身实现 |
| Handler | [internal/shop/order/http_handler.go:36](../../internal/shop/order/http_handler.go#L36) | Handler 只保存服务依赖，每个请求的参数都使用局部变量 | orderservice.routes |
| NewHandler | [internal/shop/order/http_handler.go:39](../../internal/shop/order/http_handler.go#L39) | NewHandler 在 app 组装路由时调用一次 | orderservice.routes、HTTP 测试 |
| *Handler.Create | [internal/shop/order/http_handler.go:42](../../internal/shop/order/http_handler.go#L42) | Create 不接受多个幂等键，避免代理与服务对“到底用哪一个”产生不同理解 | Gin POST /orders 路由 |
| *Handler.Get | [internal/shop/order/http_handler.go:63](../../internal/shop/order/http_handler.go#L63) | Get 使用 ParseInt 拒绝溢出的编号，不能把它截断后查询另一个订单 | Gin GET /orders/:id 路由 |
| *Handler.List | [internal/shop/order/http_handler.go:78](../../internal/shop/order/http_handler.go#L78) | List 使用契约中的默认页码和页大小，不接受无法解析的字符串 | Gin GET /orders 路由 |
| TestQueryHTTP | [internal/shop/order/http_handler_test.go:116](../../internal/shop/order/http_handler_test.go#L116) | TestQueryHTTP 验证默认分页与无效查询，空订单列表必须输出数组 | go test |
| stubHTTPService | [internal/shop/order/http_handler_test.go:30](../../internal/shop/order/http_handler_test.go#L30) | stubHTTPService 保存最后的输入，便于证明 HTTP 参数传到了业务层 | 本包测试函数及其注入的业务接口 |
| *stubHTTPService.Create | [internal/shop/order/http_handler_test.go:39](../../internal/shop/order/http_handler_test.go#L39) | Create 返回稳定的结果，错误文本故意带内部字样，检查响应没有泄露它 | 本包测试函数及其注入的业务接口 |
| *stubHTTPService.Get | [internal/shop/order/http_handler_test.go:46](../../internal/shop/order/http_handler_test.go#L46) | Get 在查询测试中只提供编号，其他详情字段由真实仓储测试覆盖 | 本包测试函数及其注入的业务接口 |
| *stubHTTPService.List | [internal/shop/order/http_handler_test.go:52](../../internal/shop/order/http_handler_test.go#L52) | List 借用真实 service 的参数校验，再返回空列表，避免替身把非法分页也当成成功 | 本包测试函数及其注入的业务接口 |
| handlerRouter | [internal/shop/order/http_handler_test.go:60](../../internal/shop/order/http_handler_test.go#L60) | handlerRouter 使用内存调用 ServeHTTP，不启动服务器或连接外部服务 | 本包测试函数及其注入的业务接口 |
| TestCreateHTTP | [internal/shop/order/http_handler_test.go:72](../../internal/shop/order/http_handler_test.go#L72) | TestCreateHTTP 覆盖幂等三种响应和严格 JSON，保证客户端能区分等一等与人工核对 | go test |
| Reservation | [internal/shop/order/idempotency.go:25](../../internal/shop/order/idempotency.go#L25) | Reservation 保存为 JSON | IdempotencyStore 的操作、Service |
| IdempotencyStore | [internal/shop/order/idempotency.go:39](../../internal/shop/order/idempotency.go#L39) | IdempotencyStore 的 bool 返回值说明是否由本次调用取得处理权，不能仅凭查不到就开始下单 | Service；RedisIdempotency 实现 |
| validKey | [internal/shop/order/idempotency.go:47](../../internal/shop/order/idempotency.go#L47) | validKey 只接受契约列出的 ASCII 字符 | Handler.Create、Service.Create |
| CreateRequest | [internal/shop/order/model.go:18](../../internal/shop/order/model.go#L18) | CreateRequest 只接受正整数，数量范围与库存 RPC 的 int32 一致 | Handler.Create、Service.Create、幂等参数摘要 |
| Created | [internal/shop/order/model.go:25](../../internal/shop/order/model.go#L25) | Created 保存在幂等记录中，同一键重放时继续使用这份结果 | Service.Create、幂等完成结果、HTTP 响应 |
| Order | [internal/shop/order/model.go:32](../../internal/shop/order/model.go#L32) | Order 是数据库中的完整订单快照，不直接暴露 sql.Rows 等驱动类型 | Repository、Service、订单详情响应 |
| Page | [internal/shop/order/model.go:43](../../internal/shop/order/model.go#L43) | Page 的 Items 在没有结果时也返回空数组，不返回 JSON null | Repository.List、Service.List、HTTP 列表响应 |
| Error | [internal/shop/order/model.go:50](../../internal/shop/order/model.go#L50) | Error 是可比较的业务分类，调用者使用 errors.Is 判断 | 领域错误常量与 errors.Is |
| Error.Error | [internal/shop/order/model.go:53](../../internal/shop/order/model.go#L53) | Error 让具名字符串可以作为标准 error 返回 | 标准 error 接口 |
| PersistenceError | [internal/shop/order/model.go:65](../../internal/shop/order/model.go#L65) | PersistenceError 区分“事务尚未提交”与“提交答复丢失”，后者不能立即归还库存 | MySQLRepository.Create；Service 读取结果确定性 |
| *PersistenceError.Error | [internal/shop/order/model.go:71](../../internal/shop/order/model.go#L71) | Error 返回具体失败原因，HTTP 层仍只输出安全的固定消息 | 标准 error 接口 |
| *PersistenceError.Unwrap | [internal/shop/order/model.go:74](../../internal/shop/order/model.go#L74) | Unwrap 让标准错误匹配能够穿过这一层包装 | errors.Is、errors.As |
| *PersistenceError.OutcomeUnknown | [internal/shop/order/model.go:77](../../internal/shop/order/model.go#L77) | OutcomeUnknown 让业务流程用同一个小接口判断库存写入与订单写入的不确定性 | outcomeUnknown |
| MySQLRepository | [internal/shop/order/repository.go:34](../../internal/shop/order/repository.go#L34) | MySQLRepository 只借用连接池 | orderservice.Run 创建；Service 借用 |
| NewMySQLRepository | [internal/shop/order/repository.go:37](../../internal/shop/order/repository.go#L37) | NewMySQLRepository 让订单与商品查询复用服务级连接池，不为每个包创建新池 | orderservice.Run、真实 MySQL 测试 |
| *MySQLRepository.Create | [internal/shop/order/repository.go:42](../../internal/shop/order/repository.go#L42) | Create 即使提交响应丢失也返回已分配的编号，人工核对可以按这个编号查数据库 | Service.Create |
| rowScanner | [internal/shop/order/repository.go:70](../../internal/shop/order/repository.go#L70) | rowScanner 同时适用于 sql.Row 和 sql.Rows，复用扫描顺序但不创建通用仓储框架 | scanOrder；sql.Row 与 sql.Rows 实现 |
| scanOrder | [internal/shop/order/repository.go:73](../../internal/shop/order/repository.go#L73) | scanOrder 按 SELECT 的固定列顺序填充模型，时间由连接配置保证为 UTC | MySQLRepository.Get、List |
| *MySQLRepository.Get | [internal/shop/order/repository.go:80](../../internal/shop/order/repository.go#L80) | Get 只有确实没有记录时返回 ErrNotFound，不把数据库故障伪装成订单不存在 | Service.Get |
| *MySQLRepository.List | [internal/shop/order/repository.go:92](../../internal/shop/order/repository.go#L92) | List 关闭 Rows 后连接才能及时回到池里 | Service.List |
| integrationDatabase | [internal/shop/order/repository_integration_test.go:27](../../internal/shop/order/repository_integration_test.go#L27) | integrationDatabase 连接前检查数据库名称，清理只发生在明确的测试库 | 本包测试函数及其注入的业务接口 |
| TestIntegrationOrderQueries | [internal/shop/order/repository_integration_test.go:57](../../internal/shop/order/repository_integration_test.go#L57) | TestIntegrationOrderQueries 从真实写入结果查询，避免只验证一套手工准备的数据 | go test |
| TestIntegrationOrderRejectedBeforeCommit | [internal/shop/order/repository_integration_test.go:94](../../internal/shop/order/repository_integration_test.go#L94) | TestIntegrationOrderRejectedBeforeCommit 让 CHECK 约束拒绝负数量，此时还没有发送 COMMIT | go test |
| *Service.safeFailure | [internal/shop/order/service.go:152](../../internal/shop/order/service.go#L152) | safeFailure 只接收已确认没有留下订单或库存变化的失败 | Service.Create 的确定失败分支 |
| *Service.unknown | [internal/shop/order/service.go:166](../../internal/shop/order/service.go#L166) | unknown 尽力保存待核对状态 | Service.Create、safeFailure |
| outcomeUnknown | [internal/shop/order/service.go:180](../../internal/shop/order/service.go#L180) | outcomeUnknown 遇到未提供确定性说明的写错误时保守处理，避免新适配器漏写说明就允许重复扣减 | Service.Create 判断写错误 |
| *Service.Get | [internal/shop/order/service.go:189](../../internal/shop/order/service.go#L189) | Get 在访问数据库前挡住无效编号 | Handler.Get |
| *Service.List | [internal/shop/order/service.go:197](../../internal/shop/order/service.go#L197) | List 限制每页数量并检查偏移量乘法，避免整数溢出后查错页 | Handler.List |
| Repository | [internal/shop/order/service.go:31](../../internal/shop/order/service.go#L31) | Repository 返回可能已分配编号的订单，即使提交答复丢失，日志也能保留这个线索 | Service；MySQLRepository 与测试替身实现 |
| PriceReader | [internal/shop/order/service.go:38](../../internal/shop/order/service.go#L38) | PriceReader 共用商品缓存，但不读取用于展示的库存状态 | Service；product.Service 实现 |
| StockWriter | [internal/shop/order/service.go:43](../../internal/shop/order/service.go#L43) | StockWriter 只暴露订单需要的扣减和补偿，二者都不应自动重试 | Service；inventoryclient.Client 实现 |
| Publisher | [internal/shop/order/service.go:49](../../internal/shop/order/service.go#L49) | Publisher 同步告知消息发送结果 | Service；events.Publisher 实现 |
| ReviewRecorder | [internal/shop/order/service.go:54](../../internal/shop/order/service.go#L54) | ReviewRecorder 同时写告警日志和计数 | Service；orderservice.reviewRecorder 实现 |
| Service | [internal/shop/order/service.go:59](../../internal/shop/order/service.go#L59) | Service 只保存依赖和固定配置，每次下单的中间结果放在局部变量中 | orderservice.Run 创建；Handler 调用 |
| NewService | [internal/shop/order/service.go:71](../../internal/shop/order/service.go#L71) | NewService 由 app 统一传入依赖，cleanupTimeout 必须大于库存调用超时 | orderservice.Run、业务测试 |
| *Service.Create | [internal/shop/order/service.go:76](../../internal/shop/order/service.go#L76) | Create 遇到结果不明就停止自动处理，不能把“没有收到成功答复”当成“什么都没发生” | Handler.Create、业务测试 |
| fakeReviews | [internal/shop/order/service_test.go:105](../../internal/shop/order/service_test.go#L105) | fakeReviews 统计进入待核对的次数，重复读取同一状态不应重复计数 | 本包测试函数及其注入的业务接口 |
| *fakeReviews.RecordReview | [internal/shop/order/service_test.go:108](../../internal/shop/order/service_test.go#L108) | RecordReview 只记录次数，真实日志字段与指标在 app 层测试 | 本包测试函数及其注入的业务接口 |
| memoryIdempotency | [internal/shop/order/service_test.go:113](../../internal/shop/order/service_test.go#L113) | memoryIdempotency 用锁模拟 Redis 的原子占位，状态与服务的公开接口保持一致 | 本包测试函数及其注入的业务接口 |
| *memoryIdempotency.Reserve | [internal/shop/order/service_test.go:122](../../internal/shop/order/service_test.go#L122) | Reserve 让首次请求拿到处理权，其他请求只返回已有状态 | 本包测试函数及其注入的业务接口 |
| *memoryIdempotency.Complete | [internal/shop/order/service_test.go:140](../../internal/shop/order/service_test.go#L140) | Complete 保存结果，后面的相同请求直接回放 | 本包测试函数及其注入的业务接口 |
| *memoryIdempotency.Abandon | [internal/shop/order/service_test.go:151](../../internal/shop/order/service_test.go#L151) | Abandon 模拟安全失败后的删除，后续请求因此可以重新占位 | 本包测试函数及其注入的业务接口 |
| *memoryIdempotency.Review | [internal/shop/order/service_test.go:159](../../internal/shop/order/service_test.go#L159) | Review 保留记录，只把状态切成不能重新执行的待核对 | 本包测试函数及其注入的业务接口 |
| fixture | [internal/shop/order/service_test.go:167](../../internal/shop/order/service_test.go#L167) | fixture 把同一组替身放在一起，让每个测试只修改与当前失败位置有关的字段 | 本包测试函数及其注入的业务接口 |
| newFixture | [internal/shop/order/service_test.go:178](../../internal/shop/order/service_test.go#L178) | newFixture 默认全部成功，失败用例只覆盖一个环节 | 本包测试函数及其注入的业务接口 |
| TestCreateFailureBoundaries | [internal/shop/order/service_test.go:185](../../internal/shop/order/service_test.go#L185) | TestCreateFailureBoundaries 检查失败之后系统留下的状态，而不只检查函数返回值 | go test |
| TestConcurrentIdempotency | [internal/shop/order/service_test.go:239](../../internal/shop/order/service_test.go#L239) | TestConcurrentIdempotency 二十个相同请求只能创建一笔订单，其他请求得到回放或处理中 | go test |
| TestCompensationSurvivesCancellation | [internal/shop/order/service_test.go:267](../../internal/shop/order/service_test.go#L267) | TestCompensationSurvivesCancellation 请求取消不应阻止已经扣下的库存被归还 | go test |
| TestValidateCreate | [internal/shop/order/service_test.go:289](../../internal/shop/order/service_test.go#L289) | TestValidateCreate 检查整数上界和键格式，所有拒绝都发生在扣库存之前 | go test |
| fakeRepository | [internal/shop/order/service_test.go:33](../../internal/shop/order/service_test.go#L33) | fakeRepository 记录创建次数，错误也会返回一个编号，模拟 INSERT 已获得主键的情况 | 本包测试函数及其注入的业务接口 |
| *fakeRepository.Create | [internal/shop/order/service_test.go:40](../../internal/shop/order/service_test.go#L40) | Create 保留 service 算出的金额，测试才能检查有没有整数计算错误 | 本包测试函数及其注入的业务接口 |
| *fakeRepository.Get | [internal/shop/order/service_test.go:50](../../internal/shop/order/service_test.go#L50) | Get 在这些流程测试里不使用，固定返回不存在 | 本包测试函数及其注入的业务接口 |
| *fakeRepository.List | [internal/shop/order/service_test.go:53](../../internal/shop/order/service_test.go#L53) | List 保留空数组的契约，流程测试不需要另建查询数据 | 本包测试函数及其注入的业务接口 |
| fakePrice | [internal/shop/order/service_test.go:58](../../internal/shop/order/service_test.go#L58) | fakePrice 固定返回测试价格或读取错误 | 本包测试函数及其注入的业务接口 |
| *fakePrice.Get | [internal/shop/order/service_test.go:64](../../internal/shop/order/service_test.go#L64) | Get 不读取库存，证明下单不会依赖商品详情中的降级结果 | 本包测试函数及其注入的业务接口 |
| fakeStock | [internal/shop/order/service_test.go:69](../../internal/shop/order/service_test.go#L69) | fakeStock 把扣减和补偿分别计数，防止“返回了错误但仍多扣一次”被漏掉 | 本包测试函数及其注入的业务接口 |
| *fakeStock.Deduct | [internal/shop/order/service_test.go:78](../../internal/shop/order/service_test.go#L78) | Deduct 不自行重试，测试关注的是 service 有没有重复调用 | 本包测试函数及其注入的业务接口 |
| *fakeStock.Release | [internal/shop/order/service_test.go:84](../../internal/shop/order/service_test.go#L84) | Release 在回调里检查 context 后，再返回预设结果 | 本包测试函数及其注入的业务接口 |
| fakePublisher | [internal/shop/order/service_test.go:93](../../internal/shop/order/service_test.go#L93) | fakePublisher 记录发送次数，发送失败不应让订单再创建一次 | 本包测试函数及其注入的业务接口 |
| *fakePublisher.Publish | [internal/shop/order/service_test.go:99](../../internal/shop/order/service_test.go#L99) | Publish 不处理消息内容，因为字段编码由 events 包的测试负责 | 本包测试函数及其注入的业务接口 |

#### internal/shop/product

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| RedisCache | [internal/shop/product/cache.go:34](../../internal/shop/product/cache.go#L34) | RedisCache 构造后只读 jitter，每次 Set 单独计算随机时间 | orderservice.Run 创建；Service 借用 |
| NewRedisCache | [internal/shop/product/cache.go:40](../../internal/shop/product/cache.go#L40) | NewRedisCache 把配置错误留在启动阶段，不让请求走到一半才失败 | orderservice.Run、缓存测试 |
| *RedisCache.Get | [internal/shop/product/cache.go:48](../../internal/shop/product/cache.go#L48) | Get 遇到损坏的 JSON 也按未命中处理，让数据库有机会修复这条缓存 | Service.Get、load |
| decodeProduct | [internal/shop/product/cache.go:60](../../internal/shop/product/cache.go#L60) | decodeProduct 验证缓存编号，避免错误写入的值让一个商品返回另一个商品的价格 | RedisCache.Get |
| *RedisCache.Set | [internal/shop/product/cache.go:75](../../internal/shop/product/cache.go#L75) | Set 用 JSON null 表示确实查不到，后续请求就不必反复查询同一个不存在的编号 | Service.load |
| *RedisCache.expiration | [internal/shop/product/cache.go:88](../../internal/shop/product/cache.go#L88) | expiration 对空值始终使用三十秒 | RedisCache.Set、TTL 测试 |
| TestIntegrationRedisCache | [internal/shop/product/cache_integration_test.go:24](../../internal/shop/product/cache_integration_test.go#L24) | TestIntegrationRedisCache 先写正常商品再覆盖为空值，两种状态都必须能被下一次读取识别 | go test |
| TestDecodeProduct | [internal/shop/product/cache_test.go:20](../../internal/shop/product/cache_test.go#L20) | TestDecodeProduct 防止损坏内容被当成价格为零的有效商品 | go test |
| TestExpiration | [internal/shop/product/cache_test.go:43](../../internal/shop/product/cache_test.go#L43) | TestExpiration 多取几次随机值检查范围，不要求随机数恰好出现某个特定值 | go test |
| Handler | [internal/shop/product/http_handler.go:24](../../internal/shop/product/http_handler.go#L24) | Handler 只保存业务服务，请求参数留在每次调用的局部变量中 | orderservice.routes |
| NewHandler | [internal/shop/product/http_handler.go:27](../../internal/shop/product/http_handler.go#L27) | NewHandler 在 app 装配路由时创建，避免每次请求重复创建服务 | orderservice.routes、HTTP 测试 |
| *Handler.Get | [internal/shop/product/http_handler.go:30](../../internal/shop/product/http_handler.go#L30) | Get 拒绝非正整数编号，避免无意义的输入继续占用数据库和 RPC 资源 | Gin GET /products/:id 路由 |
| TestProductHTTP | [internal/shop/product/http_handler_test.go:23](../../internal/shop/product/http_handler_test.go#L23) | TestProductHTTP 防止把查询超时混成 500，也防止降级时把未知库存写成零 | go test |
| Product | [internal/shop/product/model.go:16](../../internal/shop/product/model.go#L16) | Product 的价格使用整数分，避免浮点计算把金额算出小数误差 | 价格查询、缓存、仓储与 Detail |
| StockView | [internal/shop/product/model.go:23](../../internal/shop/product/model.go#L23) | StockView 用指针区分“库存是零”和“没有查到库存数值” | Service.Detail、HTTP 响应 |
| Detail | [internal/shop/product/model.go:29](../../internal/shop/product/model.go#L29) | Detail 通过匿名嵌入 Product 复用商品字段，JSON 编码时这些字段仍位于最外层 | Service.Detail 返回；Handler 输出 |
| Error | [internal/shop/product/model.go:35](../../internal/shop/product/model.go#L35) | Error 是可比较的字符串类型，业务层用 errors.Is 判断它 | 领域错误常量与 errors.Is |
| Error.Error | [internal/shop/product/model.go:38](../../internal/shop/product/model.go#L38) | Error 让具名字符串满足 Go 的 error 接口 | 标准 error 接口 |
| MySQLRepository | [internal/shop/product/repository.go:24](../../internal/shop/product/repository.go#L24) | MySQLRepository 不创建连接池，也不关闭它 | orderservice.Run 创建；Service 借用 |
| NewMySQLRepository | [internal/shop/product/repository.go:27](../../internal/shop/product/repository.go#L27) | NewMySQLRepository 复用已有连接池，避免每个业务包重复占用数据库连接 | orderservice.Run |
| *MySQLRepository.Get | [internal/shop/product/repository.go:32](../../internal/shop/product/repository.go#L32) | Get 只在 sql.ErrNoRows 时返回不存在，其他错误保留给调用方判断 | Service.load |
| *Service.Detail | [internal/shop/product/service.go:103](../../internal/shop/product/service.go#L103) | Detail 允许库存展示降级 | Handler.Get |
| Repository | [internal/shop/product/service.go:29](../../internal/shop/product/service.go#L29) | Repository 由使用者定义，测试可以换成内存实现，不必真的连接 MySQL | Service；MySQLRepository 与测试替身实现 |
| Cache | [internal/shop/product/service.go:34](../../internal/shop/product/service.go#L34) | Cache 的 hit 表示缓存里有记录 | Service；RedisCache 与内存缓存实现 |
| StockReader | [internal/shop/product/service.go:40](../../internal/shop/product/service.go#L40) | StockReader 只暴露查库存，商品详情不需要扣减能力 | Service；inventoryclient.Client 实现 |
| Service | [internal/shop/product/service.go:45](../../internal/shop/product/service.go#L45) | Service 保存商品查询依赖 | orderservice.Run 创建；Handler 和 order 使用 |
| NewService | [internal/shop/product/service.go:55](../../internal/shop/product/service.go#L55) | NewService 接收已经装配好的依赖，让测试和真实服务使用相同的业务流程 | orderservice.Run、商品测试 |
| *Service.Get | [internal/shop/product/service.go:60](../../internal/shop/product/service.go#L60) | Get 合并的是一次回源过程 | Service.Detail、order.Service.Create |
| *Service.load | [internal/shop/product/service.go:87](../../internal/shop/product/service.go#L87) | load 在真正回源前再查缓存，处理“刚准备回源，前一个请求已经写好缓存”的情况 | Service.Get 的合并回调或直接路径 |
| quietLogger | [internal/shop/product/service_test.go:103](../../internal/shop/product/service_test.go#L103) | quietLogger 避免预期故障日志淹没测试断言 | 本包测试函数及其注入的业务接口 |
| TestCacheAside | [internal/shop/product/service_test.go:106](../../internal/shop/product/service_test.go#L106) | TestCacheAside 连续查两次，第二次是否访问数据库由第一次得到的结果决定 | go test |
| TestSingleflight | [internal/shop/product/service_test.go:137](../../internal/shop/product/service_test.go#L137) | TestSingleflight 让五十个请求同时遇到冷缓存，防护开启时数据库只处理一份查询 | go test |
| TestDetailDegradation | [internal/shop/product/service_test.go:175](../../internal/shop/product/service_test.go#L175) | TestDetailDegradation 从 JSON 结果检查降级，避免只看 Go 结构体而漏掉契约问题 | go test |
| waitUntil | [internal/shop/product/service_test.go:203](../../internal/shop/product/service_test.go#L203) | waitUntil 等待可观察的条件，限定五秒，避免并发错误把测试永久挂住 | 本包测试函数及其注入的业务接口 |
| cachedValue | [internal/shop/product/service_test.go:32](../../internal/shop/product/service_test.go#L32) | cachedValue 同时保存正常值和不存在标记，模拟 Redis 中的 JSON 与 null | 本包测试函数及其注入的业务接口 |
| memoryCache | [internal/shop/product/service_test.go:38](../../internal/shop/product/service_test.go#L38) | memoryCache 用锁保护 map，因为 singleflight 测试会同时读写它 | 本包测试函数及其注入的业务接口 |
| *memoryCache.Get | [internal/shop/product/service_test.go:47](../../internal/shop/product/service_test.go#L47) | Get 和真实缓存一样区分“没有记录”和“记录说商品不存在” | 本包测试函数及其注入的业务接口 |
| *memoryCache.Set | [internal/shop/product/service_test.go:62](../../internal/shop/product/service_test.go#L62) | Set 记录是否回填 | 本包测试函数及其注入的业务接口 |
| fakeRepository | [internal/shop/product/service_test.go:74](../../internal/shop/product/service_test.go#L74) | fakeRepository 可以挡住查询，制造多个请求同时等待同一个商品的情况 | 本包测试函数及其注入的业务接口 |
| *fakeRepository.Get | [internal/shop/product/service_test.go:82](../../internal/shop/product/service_test.go#L82) | Get 等待时也尊重取消，防止测试结束后仍有查询留在后台 | 本包测试函数及其注入的业务接口 |
| fakeStock | [internal/shop/product/service_test.go:95](../../internal/shop/product/service_test.go#L95) | fakeStock 只为商品展示提供库存结果，和数据库替身互不依赖 | 本包测试函数及其注入的业务接口 |
| fakeStock.GetStock | [internal/shop/product/service_test.go:98](../../internal/shop/product/service_test.go#L98) | GetStock 正常时返回零，专门验证“零库存”不会因 omitempty 消失 | 本包测试函数及其注入的业务接口 |

#### internal/shop/sales

| 名称 | 位置（文件:行号） | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| Summary | [internal/shop/sales/model.go:13](../../internal/shop/sales/model.go#L13) | Summary 是一次数据库快照里的总量 | MySQLRepository.Check 返回；shopctl.writeCheck 输出 |
| MySQLRepository | [internal/shop/sales/repository.go:38](../../internal/shop/sales/repository.go#L38) | MySQLRepository 复用服务级连接池 | orderworker.Run、shopctl.runCheck 创建 |
| NewMySQLRepository | [internal/shop/sales/repository.go:41](../../internal/shop/sales/repository.go#L41) | NewMySQLRepository 不执行查询，启动时的探活和迁移由 app 完成 | orderworker.Run、shopctl.runCheck、真实 MySQL 测试 |
| *MySQLRepository.Apply | [internal/shop/sales/repository.go:46](../../internal/shop/sales/repository.go#L46) | Apply 的 result 是具名返回值 | Service.Handle |
| *MySQLRepository.Check | [internal/shop/sales/repository.go:78](../../internal/shop/sales/repository.go#L78) | Check 只查询总量，不修改任何订单或销量，也不会把差值自动修正成零 | shopctl.runCheck |
| TestIntegrationCheckSales | [internal/shop/sales/repository_integration_test.go:118](../../internal/shop/sales/repository_integration_test.go#L118) | TestIntegrationCheckSales 用一笔三件的订单证明检查的是件数，空表也应返回零 | go test |
| integrationDatabase | [internal/shop/sales/repository_integration_test.go:28](../../internal/shop/sales/repository_integration_test.go#L28) | integrationDatabase 在连接前校验专用库名，拒绝把清理语句发给业务库 | 本包测试函数及其注入的业务接口 |
| TestIntegrationDuplicateSales | [internal/shop/sales/repository_integration_test.go:60](../../internal/shop/sales/repository_integration_test.go#L60) | TestIntegrationDuplicateSales 用真实唯一键让二十条相同消息竞争，同一订单只能记一次账 | go test |
| TestIntegrationSalesRollback | [internal/shop/sales/repository_integration_test.go:90](../../internal/shop/sales/repository_integration_test.go#L90) | TestIntegrationSalesRollback 故意让 BIGINT 加法溢出，验证第一条 INSERT 也被回滚 | go test |
| Repository | [internal/shop/sales/service.go:21](../../internal/shop/sales/service.go#L21) | Repository 的 Apply 必须把去重与加销量作为一件事完成，不能只成功一半 | Service；MySQLRepository 实现 |
| Service | [internal/shop/sales/service.go:26](../../internal/shop/sales/service.go#L26) | Service 只持有仓储，不直接认识 Kafka 或数据库驱动 | orderworker.Run 创建；Consumer 使用 |
| NewService | [internal/shop/sales/service.go:29](../../internal/shop/sales/service.go#L29) | NewService 由 app 把真实仓储传入，测试可以传入替身 | orderworker.Run |
| *Service.Handle | [internal/shop/sales/service.go:32](../../internal/shop/sales/service.go#L32) | Handle 只有在数据库处理成功后才返回 nil，消费者据此决定是否提交位点 | events.Consumer.process 经 handle 回调 |
<!-- SYMBOLS_END -->

### 3.4 数据库变更

没有修改阶段 1 的五份业务迁移。复用 products、stock、orders、product_sales、processed_events。新增的 `003_order_sales_test_databases.sql` 只创建订单与销量的隔离测试库并授权，不保存密码。

### 3.5 接口与端口

新增 GET /products/:id、POST /orders、GET /orders/:id、GET /orders。HTTP 业务端口 8081，订单管理 6061，消费者管理 6063。loadgen 的 6070 提供 GET/POST /control 和 GET /health。6061 的 GET /metrics 仅导出明确要求的待核对计数。

库存三个 RPC 没有修改。幂等新增 409 REQUEST_OUTCOME_UNKNOWN；同键改参数返回 400。处理中、完成回放、结果待核对是三种独立状态。完整格式见计划书第 5.8.2 节。

### 3.6 配置变更

| 变量（新增或本阶段正式启用） | 含义 | 程序默认值 / 测试要求 |
|---|---|---|
| HTTP_ADDR / ADMIN_ADDR | 业务与管理监听 | 订单 :8081 / :6061，worker 管理 :6063 |
| REDIS_ADDR / KAFKA_BROKERS | 缓存和消息地址 | 127.0.0.1:6379 / 127.0.0.1:9092；Compose 使用服务名 |
| INVENTORY_GRPC_TARGET | 库存 RPC 地址 | 127.0.0.1:9001；Compose 使用 inventory-service:9001 |
| HTTP_REQUEST_TIMEOUT / ORDER_CLEANUP_TIMEOUT | 请求与每次收尾预算 | 3s / 2s |
| INVENTORY_CALL_TIMEOUT | 一次逻辑库存调用总预算 | 800ms |
| INVENTORY_GETSTOCK_MAX_RETRIES | 首次查询以外的尝试次数 | 2；扣减与归还不使用它 |
| INVENTORY_RETRY_BACKOFF / INVENTORY_RETRY_MIN_BUDGET | 初始退避 / 下一次至少剩余时间 | 50ms / 50ms |
| INVENTORY_BREAKER_ENABLED | 开关 | true |
| INVENTORY_BREAKER_MIN_REQUESTS / INVENTORY_BREAKER_FAILURE_PERCENT | 最少样本 / 失败百分比 | 10 / 50 |
| INVENTORY_BREAKER_INTERVAL / INVENTORY_BREAKER_OPEN_TIMEOUT / INVENTORY_BREAKER_HALF_OPEN_MAX | 计数窗口 / 打开等待 / 半开额度 | 10s / 5s / 1 |
| RATE_LIMIT_RPS / RATE_LIMIT_BURST | 每秒补充令牌 / 桶容量 | 500 / 100 |
| CACHE_SINGLEFLIGHT / CACHE_TTL_JITTER | 合并请求 / 过期随机增量 | true / 60s；false 和 0s 可关闭 |
| IDEMPOTENCY_TTL | 请求记录保存期 | 24h，须为整秒且长于完整处理预算 |
| WORKER_MESSAGE_TIMEOUT | 当前消息处理和位点提交预算 | 10s |
| LOADGEN_TARGET / LOADGEN_ADDR | HTTP 目标 / 控制监听 | http://127.0.0.1:8081 / :6070 |
| LOADGEN_QPS / LOADGEN_PRODUCT_COUNT / LOADGEN_WORKERS | 速率 / 数据规模 / 并发上限 | 200 / 500000 / 64；本机达标轮显式用 256 |
| LOADGEN_REQUEST_TIMEOUT / LOADGEN_DURATION | 每次尝试预算 / 调度时长 | 3s / 0s（持续运行） |
| TEST_ORDER_MYSQL_DSN / TEST_SALES_MYSQL_DSN | 隔离集成测试 | 无默认值；只允许 opsdoctor_order_test / opsdoctor_sales_test |
| TEST_REDIS_ADDR | 缓存与幂等真实测试 | 无默认值；固定 DB 14/15，清理自身 key |

所有实际读取的配置都已列入计划书第 5.8.4 节和 .env.example。MYSQL_DSN 与连接池变量沿用阶段 1；真实凭据只在忽略的本机环境中。

### 3.7 部署变更

新增 order-service、order-worker、loadgen，连同已有四个容器组成完整靶场。所有宿主机端口绑定回环地址。Redis 新增持久卷、AOF 每次刷盘和 noeviction，内存不足时拒绝新写入。CI 暂停空库流量，跑隔离集成测试、small 造数、HTTP 同键回放、短压测和最终总量核对。

## 四、测试

<!-- TESTS_START -->
| 测试名 | 文件 | 验证场景 |
|---|---|---|
| TestStatistics | [internal/loadgen/loadgen_test.go](../../internal/loadgen/loadgen_test.go) | TestStatistics 用已知的一百个样本核对百分位，不把配置 QPS 直接写进报告 |
| TestControl | [internal/loadgen/loadgen_test.go](../../internal/loadgen/loadgen_test.go) | TestControl 使用内存 HTTP 请求检查部分更新，非法值不能改动现有设置 |
| TestBoundedWorkers | [internal/loadgen/loadgen_test.go](../../internal/loadgen/loadgen_test.go) | TestBoundedWorkers 让发送持续等到请求超时，确认高 QPS 也不会无限启动工作者 |
| TestTrafficDistribution | [internal/loadgen/loadgen_test.go](../../internal/loadgen/loadgen_test.go) | TestTrafficDistribution 不要求每十次恰好七二一，而是核对大量固定随机样本的比例 |
| TestRetryKeepsKey | [internal/loadgen/loadgen_test.go](../../internal/loadgen/loadgen_test.go) | TestRetryKeepsKey 检查超时重试与不该重试的响应，同时验证统计保留每次尝试 |
| TestBreakerRecovery | [internal/platform/resilience/resilience_test.go](../../internal/platform/resilience/resilience_test.go) | TestBreakerRecovery 用短窗口观察完整状态变化，确认打开期间没有偷偷执行下游回调 |
| TestRateLimit | [internal/platform/resilience/resilience_test.go](../../internal/platform/resilience/resilience_test.go) | TestRateLimit 用一个几乎不会在测试期间补满的桶，明确看到“放行一次，再拒绝一次” |
| TestRetryPolicy | [internal/platform/resilience/resilience_test.go](../../internal/platform/resilience/resilience_test.go) | TestRetryPolicy 固定随机增量，让指数退避的每一步都可以直接核对 |
| TestRetryCancellation | [internal/platform/resilience/resilience_test.go](../../internal/platform/resilience/resilience_test.go) | TestRetryCancellation 让第一次失败同时取消上下文，等待器必须立即结束 |
| TestConfigBudgets | [internal/shop/app/orderservice/config_test.go](../../internal/shop/app/orderservice/config_test.go) | TestConfigBudgets 每次只覆盖一个变量，便于知道是哪条配置规则拒绝了启动 |
| TestConnectInventory | [internal/shop/app/orderservice/connection_test.go](../../internal/shop/app/orderservice/connection_test.go) | TestConnectInventory 对照真实内存握手与持续拒绝拨号，启动不能把未就绪当成成功 |
| TestReviewRecorder | [internal/shop/app/orderservice/metrics_test.go](../../internal/shop/app/orderservice/metrics_test.go) | TestReviewRecorder 验证调用一次便同时产生告警和计数，避免接口接上了却没有实际记录 |
| TestCheckOutput | [internal/shop/app/shopctl/check_test.go](../../internal/shop/app/shopctl/check_test.go) | TestCheckOutput 使用明确的总量，让输出必须来自传入结果，而不是固定写零 |
| TestConsumerFinishesCurrentMessage | [internal/shop/events/consumer_test.go](../../internal/shop/events/consumer_test.go) | TestConsumerFinishesCurrentMessage 在处理数据库时取消服务，当前消息的独立预算仍然有效 |
| TestRejectInvalidMessage | [internal/shop/events/consumer_test.go](../../internal/shop/events/consumer_test.go) | TestRejectInvalidMessage 坏消息不提交，避免静默丢弃后看起来消费一切正常 |
| TestConsumerCommitOrder | [internal/shop/events/consumer_test.go](../../internal/shop/events/consumer_test.go) | TestConsumerCommitOrder 分别模拟数据库失败、位点提交失败和重复消息 |
| TestWriteOnceAndBreaker | [internal/shop/inventoryclient/inventory_client_test.go](../../internal/shop/inventoryclient/inventory_client_test.go) | TestWriteOnceAndBreaker 业务拒绝不能熔断 |
| TestCanceledRequest | [internal/shop/inventoryclient/inventory_client_test.go](../../internal/shop/inventoryclient/inventory_client_test.go) | TestCanceledRequest 确认请求还没发出时就取消，不会留下“可能已经扣减”的标记 |
| TestReadRetryAndDeadline | [internal/shop/inventoryclient/inventory_client_test.go](../../internal/shop/inventoryclient/inventory_client_test.go) | TestReadRetryAndDeadline 同时核对成功恢复和达到次数上限，防止只测到成功路径 |
| TestIntegrationReservationOwnership | [internal/shop/order/cache_integration_test.go](../../internal/shop/order/cache_integration_test.go) | TestIntegrationReservationOwnership 删除安全失败的占位后，新请求得到新的所有者，旧请求不能再修改它 |
| TestIntegrationExpiredProcessing | [internal/shop/order/cache_integration_test.go](../../internal/shop/order/cache_integration_test.go) | TestIntegrationExpiredProcessing 模拟进程退出留下的占位，不用真实等待十秒 |
| TestIntegrationReservationCompetition | [internal/shop/order/cache_integration_test.go](../../internal/shop/order/cache_integration_test.go) | TestIntegrationReservationCompetition 验证 SET NX 的竞争结果，再检查写回结果是否保留原 TTL |
| TestQueryHTTP | [internal/shop/order/http_handler_test.go](../../internal/shop/order/http_handler_test.go) | TestQueryHTTP 验证默认分页与无效查询，空订单列表必须输出数组 |
| TestCreateHTTP | [internal/shop/order/http_handler_test.go](../../internal/shop/order/http_handler_test.go) | TestCreateHTTP 覆盖幂等三种响应和严格 JSON，保证客户端能区分等一等与人工核对 |
| TestIntegrationOrderQueries | [internal/shop/order/repository_integration_test.go](../../internal/shop/order/repository_integration_test.go) | TestIntegrationOrderQueries 从真实写入结果查询，避免只验证一套手工准备的数据 |
| TestIntegrationOrderRejectedBeforeCommit | [internal/shop/order/repository_integration_test.go](../../internal/shop/order/repository_integration_test.go) | TestIntegrationOrderRejectedBeforeCommit 让 CHECK 约束拒绝负数量，此时还没有发送 COMMIT |
| TestCreateFailureBoundaries | [internal/shop/order/service_test.go](../../internal/shop/order/service_test.go) | TestCreateFailureBoundaries 检查失败之后系统留下的状态，而不只检查函数返回值 |
| TestConcurrentIdempotency | [internal/shop/order/service_test.go](../../internal/shop/order/service_test.go) | TestConcurrentIdempotency 二十个相同请求只能创建一笔订单，其他请求得到回放或处理中 |
| TestCompensationSurvivesCancellation | [internal/shop/order/service_test.go](../../internal/shop/order/service_test.go) | TestCompensationSurvivesCancellation 请求取消不应阻止已经扣下的库存被归还 |
| TestValidateCreate | [internal/shop/order/service_test.go](../../internal/shop/order/service_test.go) | TestValidateCreate 检查整数上界和键格式，所有拒绝都发生在扣库存之前 |
| TestIntegrationRedisCache | [internal/shop/product/cache_integration_test.go](../../internal/shop/product/cache_integration_test.go) | TestIntegrationRedisCache 先写正常商品再覆盖为空值，两种状态都必须能被下一次读取识别 |
| TestDecodeProduct | [internal/shop/product/cache_test.go](../../internal/shop/product/cache_test.go) | TestDecodeProduct 防止损坏内容被当成价格为零的有效商品 |
| TestExpiration | [internal/shop/product/cache_test.go](../../internal/shop/product/cache_test.go) | TestExpiration 多取几次随机值检查范围，不要求随机数恰好出现某个特定值 |
| TestProductHTTP | [internal/shop/product/http_handler_test.go](../../internal/shop/product/http_handler_test.go) | TestProductHTTP 防止把查询超时混成 500，也防止降级时把未知库存写成零 |
| TestCacheAside | [internal/shop/product/service_test.go](../../internal/shop/product/service_test.go) | TestCacheAside 连续查两次，第二次是否访问数据库由第一次得到的结果决定 |
| TestSingleflight | [internal/shop/product/service_test.go](../../internal/shop/product/service_test.go) | TestSingleflight 让五十个请求同时遇到冷缓存，防护开启时数据库只处理一份查询 |
| TestDetailDegradation | [internal/shop/product/service_test.go](../../internal/shop/product/service_test.go) | TestDetailDegradation 从 JSON 结果检查降级，避免只看 Go 结构体而漏掉契约问题 |
| TestIntegrationCheckSales | [internal/shop/sales/repository_integration_test.go](../../internal/shop/sales/repository_integration_test.go) | TestIntegrationCheckSales 用一笔三件的订单证明检查的是件数，空表也应返回零 |
| TestIntegrationDuplicateSales | [internal/shop/sales/repository_integration_test.go](../../internal/shop/sales/repository_integration_test.go) | TestIntegrationDuplicateSales 用真实唯一键让二十条相同消息竞争，同一订单只能记一次账 |
| TestIntegrationSalesRollback | [internal/shop/sales/repository_integration_test.go](../../internal/shop/sales/repository_integration_test.go) | TestIntegrationSalesRollback 故意让 BIGINT 加法溢出，验证第一条 INSERT 也被回滚 |
<!-- TESTS_END -->

最终本地命令：`gofmt -l cmd gen internal tools`、`go mod tidy`、`go vet ./...`、`staticcheck ./...`、`go run ./tools/commentcheck ./...`、`go run ./tools/archcheck ./...`、`go test -race ./... -count=1 -timeout=180s`。全部退出 0。格式、vet、staticcheck 无诊断；commentcheck 通过 132 个非生成 Go 文件，archcheck 通过。

未注入依赖时，67 个顶层测试通过，16 个真实集成测试按约定跳过。注入六个专用 MySQL DSN 和 TEST_REDIS_ADDR 后，83 个顶层测试全部通过、0 跳过、23 个有测试的包通过。新增 40 个顶层测试，表格子用例不重复算作顶层测试。九个 Go 示例补上必要上下文后均编译通过；七段 PowerShell、CI YAML 和两段内嵌 Python 语法检查通过。

详细原始数值和失败记录见 [notes](../notes.md#phase-02)。[Linux CI 39961041773](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/39961041773) 已通过，验证提交 4a467008d4da5968342523cf43530b879433ae94；后续仅补充文档记录，非 Markdown 源码指纹不变。

## 五、工程验收结果

| 验收项 | 结果 | 证据 |
|---|---|---|
| 一条 Compose 命令启动整个靶场 | 通过 | `up -d --build --wait --wait-timeout 240`，七容器健康，流量暂停 |
| 200 QPS 持续五分钟，错误率 <1%，记录百分位 | 通过 | 内部网络、256 工作者；300.011s、59998 请求、199.986 QPS、0 错误、0 dropped、P50=5ms、P99=51ms |
| 杀掉 worker 后追平且不重复计数 | 通过 | LAG 274/282/282→0/0/0；停流量后 2035650=2035650；另有二十次并发重复事件只加一次的真实 MySQL 测试 |
| 三个服务打印优雅退出过程 | 通过 | 库存、订单、消费者均有收到停止与入口/消费循环结束日志；订单 drain_failed=false |
| 同键提交两次仅一笔订单 | 通过 | 两次 201 都返回订单 2000001；数据库独立用户仅一笔、两件 |
| 库存停服降级、快速失败、熔断和自动恢复 | 通过 | 15 次商品降级；10 次待核对 409、5 次约 5–7ms 的 503；两熔断器 open→half-open→closed；恢复下单 201 |

压测三轮全部保留：第一轮宿主机 197.002 QPS、0 错误、880 dropped；第二轮 199.216 QPS、0.281021% 错误、302 dropped。第三轮改为内部网络且已有部分热缓存，尾段与 worker 停机实验重叠，不能据此宣称性能差异完全由网络或工作者数量造成。完整口径见 [notes](../notes.md#phase-02)。

额外验证 Redis 停机：相同完成键返回 503，保留卷重启后重新返回原订单 2035650，未新建订单。最后 `shopctl check` 为 2035651=2035651、gap=0。

## 六、偏离计划之处

原计划的“失败即删占位”不适用于写结果不确定。按 AGENTS 第 6 节先向用户说明，用户批准保留占位，并追加独立待核对错误码、详细告警和 needs_review 计数。计划书第 5.2.6、5.8.2 节已同步。

计数是用户明确授权的阶段 2 例外。没有开发完整监控管道、调用链、诊断能力或 Outbox。相关决定见 [幂等失败边界决定](../decisions.md#phase-02-idempotency)。

## 七、已知问题与遗留事项

- 根治扣库存超时的结果不确定，需要让 Deduct 自身携带请求 ID 去重。本阶段没有实现。
- 补偿可能失败；订单已写入但消息发送失败会少统计销量。本阶段没有 Outbox 自动修复。
- 结果待核对需要人工查日志、库存与订单。Redis 暂时不能写状态时，旧占位超过完整处理预算后转为待核对。
- 不带键、换键、超过保存期，或 Redis 数据卷丢失，都超出请求去重保证。单节点 Redis 不提供高可用。
- singleflight 只覆盖单进程，当前没有商品改价接口。深页码查询仍有跳过大量行的成本。
- 消息损坏会停止消费者，不静默丢弃，没有死信队列。
- loadgen 的延迟是毫秒桶上界估计，工作者容量不足会记录 dropped。目标速率不能代替实际完成速率。
- 第一次连接问题已修复；首次失败留下的待核对记录没有删除。第一次压测少发请求的结果保留，不只展示好看的结果。
- 完整指标、链路、日志采集和告警管道留在阶段 3；本阶段仅有指定计数。

## 八、注释自查结论

commentcheck 通过 132 个非生成 Go 文件。按第 3.1、3.2 节复读目录、文件、代码三层说明：分组目录有 README，包有 doc.go，文件清单包含测试文件；普通 Go 文件都有用途、主要内容和阅读入口。51 个旧文件补齐主要内容，其他旧注释的长分句也改成更自然的短句。

代码重点说明了每个失败分支已经产生的副作用、SQL 索引与事务、共享锁、工作者退出和通道所有权。把固定客户端预算、调度间隔和发送次数提成具名常量并解释取值。Go 的接口、指针、泛型、select、defer、errors.As/Join、WithoutCancel 在首次相关位置或学习材料中解释。没有保留“每个函数都并发安全、不阻塞”一类套话。

过程中 commentcheck 确实发现过长函数内部讲解不足，已补齐并重跑。工具不判断语句是否易读，因此没有把工具通过当作人工审查的替代。旧文件的 AST 与编译指令比较通过：71 个文件逻辑未变，只有 shopctl 分派按本阶段需要增加 check。地图由 `commentcheck -map` 重建，符号清单由 `-list` 生成后补齐调用方。

## 九、学习指引

进入[阶段 2 学习材料](../learn/phase-02-order.md)。建议按订单入口、业务接口、缓存与库存客户端、订单事务与幂等、消息与销量、loadgen 的顺序阅读。先用约十二小时读通路径，再根据你的进度安排实验与复盘。学习验收仍由本人勾选。

## 十、下一阶段预告

阶段 3 才开始完整可观测性与告警。本次只预告，不实现，也不自动创建下一阶段分支。暂不需要申请付费大模型接口。
