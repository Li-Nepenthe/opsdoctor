# 阶段 3 汇报书：可观测性与告警

**状态：尚未完成收尾。** 本地业务与观测管道已验证，Grafana 与 Jaeger 的版本兼容决定待用户确认；尚未推送阶段分支，远端 CI 未运行。工程验收不勾选，学习验收留给用户本人。

## 一、总览

### 一句话总结

三个业务服务已能输出统一指标、跨 Kafka 的调用链和关联日志，真实消费积压在 167.064 秒内送达临时告警接收器。

### 做了什么

新增独立的监控注册表和追踪对象，由各服务入口显式传入。用现成插件观察 HTTP、gRPC、SQL、Redis，手动传递 Kafka 消息头。Compose 从七个容器扩为十七个，固定镜像、限制内存、只向本机发布端口。看板的十二个面板、Grafana 日志查询和告警触发恢复均已实测；Grafana 的 Jaeger 数据源仍有版本兼容阻塞。按用户要求修复订单提交后客户端取消导致发送提前取消的问题。

### 计划完成情况

| 阶段 3 的“做”清单 | 状态 | 说明 |
|---|---|---|
| 三个服务的指标 | 已完成 | 统一 service 标签、HTTP/RPC、连接池、缓存、生产消费、治理、幂等、版本与运行时 |
| OTel 与 Kafka 传播 | 已完成 | 一笔真实下单得到三个服务的完整父子关系 |
| slog 关联字段 | 已完成 | 当前 span 的 trace_id/span_id；启动日志没有请求关联 |
| 九个监控组件加入 Compose | 部分完成 | 另加 alertsink，共十个新容器；Jaeger 版本兼容选择待确认 |
| Grafana 自动数据源与看板 | 部分完成 | 十二面板有数据、Loki 可查；Jaeger 数据源健康失败尚未解决 |
| 症状规则与推送 | 已完成 | 五条规则，真实积压告警与恢复均到达 |

未开始阶段 4，没有开发医生系统。

## 二、新引入的技术与依赖

| 技术或库 | 版本 | 用在哪里 | 为什么选它 | 放弃的替代方案 |
|---|---|---|---|---|
| OTel SDK / OTLP gRPC exporter | v1.47.0 | telemetry、三个业务服务 | 计划指定标准追踪，官方稳定版 | 自定义追踪协议 |
| otelgin / otelgrpc | v0.72.0 | HTTP 与 RPC 接线 | 插件处理协议头与语义 | 手工重写协议追踪 |
| XSAM/otelsql | v0.44.0 | mysqlx | 包装 sql.Connector，保留连接池使用方式 | 每条 SQL 手写 span |
| redisotel | v9.23.0 | 订单 Redis 客户端 | 与 go-redis 同版本，能关闭原始命令记录 | 把缓存正文写入 trace |
| prometheus/client_golang | v1.25.0，沿用 | telemetry、管理端口 | 统一阶段 2 的 needs_review 与全部新指标 | 多份不相通的注册表 |
| mimetype | v1.4.16，间接依赖 | 模块依赖 | 安装时 v1.4.15 已撤回，改用有效版本 | 保留被撤回版本 |
| Prometheus | v3.15.1 | 指标采集、规则评估 | 计划指定，固定稳定版 | 额外指标平台 |
| Alertmanager | v0.34.1 | 分组、去重、通知 | 计划指定 | 在业务服务里发送告警 |
| Jaeger | 当前 2.22.0 | OTLP 接收与 trace 查询 | 计划要求最新稳定版 | 兼容的 2.20.0 待用户批准 |
| Grafana | 13.2.4 | 三数据源和总览看板 | 计划指定 | 手工逐次建看板 |
| Loki | 3.7.9 | 本机日志保存 | 计划指定，保留 48 小时 | 引入其他日志存储 |
| Alloy | v1.21.2 | Docker 日志采集 | 计划指定 | 每个应用直接写 Loki |
| mysqld-exporter | v0.21.0 | MySQL 指标 | 现成采集器 | 业务服务使用 root 采集 |
| redis_exporter | v1.95.0 | Redis 指标 | 现成采集器 | 自写定时 INFO 解析 |
| kafka-exporter | v1.11.0 | 消费组 lag | 现成采集器 | 在业务里猜消费进度 |

本阶段没有安装新的命令行工具；继续使用 Go 1.27.1、staticcheck v0.8.2、Docker Compose。Go 构建镜像 `golang:1.27.1-alpine3.24`、运行镜像 `alpine:3.24.2` 沿用原版本，增加构建版本参数。完整直接和间接依赖由 go.mod/go.sum 固定。

## 三、代码明细

### 包与职责

| 包路径 | 新增 / 修改 | 职责 | 对外提供什么 |
|---|---|---|---|
| internal/platform/telemetry | 新增 | 与业务无关的指标与追踪规则 | Runtime、Metrics、协议计时、Kafka 头适配 |
| internal/platform/alertsink | 新增 | 临时接收告警 | Run、Handler；不执行诊断 |
| cmd/alertsink | 新增 | 薄入口 | 进程退出码 |
| internal/platform/adminserver | 修改 | 独立管理端口 | /metrics 和 /debug/pprof |
| internal/platform/logging | 修改 | 关联结构化日志 | 优先使用当前 span |
| internal/platform/mysqlx | 修改 | 连接池创建 | 可选注入 TracerProvider |
| internal/platform/resilience | 修改 | 熔断 | 当前状态只读查询 |
| internal/shop/app/orderservice | 修改 | 依赖装配与指标包装 | 拆分连接、业务接线；统一幂等计数 |
| internal/shop/app/inventoryservice、orderworker | 修改 | 进程装配 | 各自注册表、OTel、版本日志 |
| internal/shop/inventoryclient | 修改 | 库存调用治理 | 熔断状态和实际重试指标 |
| internal/shop/events | 修改 | Kafka 发送消费 | 手动上下文传播，成功/重复/失败指标 |
| internal/shop/sales | 修改 | 去重事务 | Apply/Handle 返回 duplicate，事务行为不变 |
| internal/shop/order | 修改 | 下单收尾 | 提交后独立预算发送 |
| internal/loadgen | 修改 | 真实流量 | 逐次 HTTP 尝试指标与管理 /metrics |

公共观测代码放 platform；商品缓存命中等业务含义留在 app 的包装里。没有新增顶层目录或 internal 一级目录，没有修改 ai-backend。

### 文件与类型函数清单

下面两张清单由当前工作区和 `commentcheck -list` 生成。提交前还需与 `git diff --stat main...phase-03-observability` 核对。文件行数为当前总行数；增删量为对 main 的差异。

<!-- PHASE_FILES_START -->
| 文件 | 作用 | 当前行数 | 对共同基线增 / 删 |
|---|---|---:|---:|
| [.env.example](../../.env.example) | 环境变量名称与演示值 | 87 | +10 / −0 |
| [.github/workflows/ci.yml](../../.github/workflows/ci.yml) | 固定 runner 上增加观测管道冒烟 | 197 | +60 / −2 |
| [AGENTS.md](../../AGENTS.md) | AGENTS.md | 115 | +1 / −1 |
| [README.md](../../README.md) | 所在目录的职责、入口与操作说明 | 267 | +42 / −9 |
| [cmd/README.md](../../cmd/README.md) | 所在目录的职责、入口与操作说明 | 15 | +2 / −0 |
| [cmd/alertsink/main.go](../../cmd/alertsink/main.go) | 命令薄入口 | 19 | +19 / −0 |
| [deploy/README.md](../../deploy/README.md) | 所在目录的职责、入口与操作说明 | 19 | +9 / −0 |
| [deploy/alertmanager/README.md](../../deploy/alertmanager/README.md) | 所在目录的职责、入口与操作说明 | 7 | +7 / −0 |
| [deploy/alertmanager/alertmanager.yml](../../deploy/alertmanager/alertmanager.yml) | 同一服务的同类症状合成一次通知，减少一场故障同时产生的大量推送。 | 18 | +18 / −0 |
| [deploy/alloy/README.md](../../deploy/alloy/README.md) | 所在目录的职责、入口与操作说明 | 7 | +7 / −0 |
| [deploy/alloy/config.alloy](../../deploy/alloy/config.alloy) | 只发现当前 Compose 项目的容器，避免顺带收集用户其他项目的日志。 | 46 | +46 / −0 |
| [deploy/docker-compose.yml](../../deploy/docker-compose.yml) | 阶段 3 共 17 个容器；内存设上限，宿主机端口只映射到本机。 | 403 | +183 / −5 |
| [deploy/docker/go.Dockerfile](../../deploy/docker/go.Dockerfile) | 所有 Go 服务共用此文件，CMD_NAME 对应 cmd/ 下的程序名；固定编译器与 Alpine 版本。 | 17 | +2 / −1 |
| [deploy/grafana/README.md](../../deploy/grafana/README.md) | 所在目录的职责、入口与操作说明 | 8 | +8 / −0 |
| [deploy/grafana/dashboards/README.md](../../deploy/grafana/dashboards/README.md) | 所在目录的职责、入口与操作说明 | 7 | +7 / −0 |
| [deploy/grafana/dashboards/shop.json](../../deploy/grafana/dashboards/shop.json) | Grafana 十二面板定义 | 478 | +478 / −0 |
| [deploy/grafana/provisioning/README.md](../../deploy/grafana/provisioning/README.md) | 所在目录的职责、入口与操作说明 | 8 | +8 / −0 |
| [deploy/grafana/provisioning/dashboards/README.md](../../deploy/grafana/provisioning/dashboards/README.md) | 所在目录的职责、入口与操作说明 | 7 | +7 / −0 |
| [deploy/grafana/provisioning/dashboards/shop.yml](../../deploy/grafana/provisioning/dashboards/shop.yml) | 看板 JSON 由仓库维护。页面中的临时调整不会覆盖版本控制里的原稿。 | 12 | +12 / −0 |
| [deploy/grafana/provisioning/datasources/README.md](../../deploy/grafana/provisioning/datasources/README.md) | 所在目录的职责、入口与操作说明 | 7 | +7 / −0 |
| [deploy/grafana/provisioning/datasources/sources.yml](../../deploy/grafana/provisioning/datasources/sources.yml) | Grafana 启动时创建三类数据源，用户不必手工填写容器内地址。 | 38 | +38 / −0 |
| [deploy/jaeger/README.md](../../deploy/jaeger/README.md) | 所在目录的职责、入口与操作说明 | 7 | +7 / −0 |
| [deploy/jaeger/jaeger.yml](../../deploy/jaeger/jaeger.yml) | 单进程内存存储适合本机演练。最多保留五千条 trace，重启后清空，不宣称长期存档。 | 42 | +42 / −0 |
| [deploy/loki/README.md](../../deploy/loki/README.md) | 所在目录的职责、入口与操作说明 | 7 | +7 / −0 |
| [deploy/loki/loki.yml](../../deploy/loki/loki.yml) | 本机单副本，日志存到独立卷。服务只映射到回环地址，不把无认证的接口开放到局域网。 | 41 | +41 / −0 |
| [deploy/mysql/init/004_metrics_user.sh](../../deploy/mysql/init/004_metrics_user.sh) | !/bin/bash | 16 | +16 / −0 |
| [deploy/mysql/init/README.md](../../deploy/mysql/init/README.md) | 所在目录的职责、入口与操作说明 | 11 | +2 / −0 |
| [deploy/prometheus/README.md](../../deploy/prometheus/README.md) | 所在目录的职责、入口与操作说明 | 8 | +8 / −0 |
| [deploy/prometheus/prometheus.yml](../../deploy/prometheus/prometheus.yml) | 十秒抓取一次足够观察本机的分钟级症状，也不会让监控本身频繁争抢数据库连接。 | 33 | +33 / −0 |
| [deploy/prometheus/rules/README.md](../../deploy/prometheus/rules/README.md) | 所在目录的职责、入口与操作说明 | 7 | +7 / −0 |
| [deploy/prometheus/rules/shop.yml](../../deploy/prometheus/rules/shop.yml) | 这里只写用户能观察到的症状。规则不能预先给出“索引被删了”之类的原因。 | 34 | +34 / −0 |
| [docs/decisions.md](../../docs/decisions.md) | 设计决定 | 155 | +36 / −0 |
| [docs/learn/phase-03-observability.md](../../docs/learn/phase-03-observability.md) | 阶段 3 学习材料：可观测性与告警 | 393 | +393 / −0 |
| [docs/notes.md](../../docs/notes.md) | 实测记录 | 378 | +68 / −0 |
| [docs/reports/phase-03-observability.md](../../docs/reports/phase-03-observability.md) | 阶段 3 汇报书：可观测性与告警 | 434 | +434 / −0 |
| [docs/代码地图.md](../../docs/代码地图.md) | OpsDoctor 代码地图 | 206 | +20 / −5 |
| [docs/数据台账.md](../../docs/数据台账.md) | 数据台账 | 70 | +17 / −0 |
| [go.mod](../../go.mod) | 固定 Go 直接和间接依赖 | 80 | +39 / −18 |
| [go.sum](../../go.sum) | 依赖内容校验和 | 199 | +85 / −37 |
| [internal/loadgen/app.go](../../internal/loadgen/app.go) | 启动压测进程，管理控制端口、统计输出和退出。 | 119 | +9 / −3 |
| [internal/loadgen/client.go](../../internal/loadgen/client.go) | 发送真实 HTTP 请求，并按约定重试超时下单。 | 100 | +7 / −0 |
| [internal/loadgen/generator.go](../../internal/loadgen/generator.go) | 按指定速率选择请求类型和商品，并交给固定工作者。 | 159 | +13 / −11 |
| [internal/loadgen/loadgen_test.go](../../internal/loadgen/loadgen_test.go) | 验证压测请求分布、重试身份和真实统计口径。 | 201 | +2 / −1 |
| [internal/platform/README.md](../../internal/platform/README.md) | 所在目录的职责、入口与操作说明 | 20 | +5 / −2 |
| [internal/platform/adminserver/doc.go](../../internal/platform/adminserver/doc.go) | 包说明、文件清单与阅读顺序 | 11 | +3 / −3 |
| [internal/platform/adminserver/server.go](../../internal/platform/adminserver/server.go) | 管理端口的健康检查和有界关闭。 | 69 | +16 / −3 |
| [internal/platform/adminserver/server_test.go](../../internal/platform/adminserver/server_test.go) | 在内存中验证健康状态和管理接口边界。 | 76 | +28 / −3 |
| [internal/platform/alertsink/app.go](../../internal/platform/alertsink/app.go) | 启动临时告警接收器，并在退出时排空已接收的请求。 | 75 | +75 / −0 |
| [internal/platform/alertsink/doc.go](../../internal/platform/alertsink/doc.go) | 包说明、文件清单与阅读顺序 | 13 | +13 / −0 |
| [internal/platform/alertsink/handler.go](../../internal/platform/alertsink/handler.go) | 阶段 3 临时接收 Alertmanager 的通知，证明告警管道已经接通。 | 80 | +80 / −0 |
| [internal/platform/alertsink/handler_test.go](../../internal/platform/alertsink/handler_test.go) | 验证告警入口只接受带正确令牌的有限通知。 | 59 | +59 / −0 |
| [internal/platform/logging/logging.go](../../internal/platform/logging/logging.go) | 在 JSON 日志中自动关联请求与服务。 | 170 | +9 / −2 |
| [internal/platform/logging/logging_test.go](../../internal/platform/logging/logging_test.go) | 检查日志字段、UTC 时间和分组之后的关联信息。 | 96 | +27 / −0 |
| [internal/platform/mysqlx/mysql.go](../../internal/platform/mysqlx/mysql.go) | 建立经过校验的 MySQL 连接池。 | 115 | +18 / −5 |
| [internal/platform/resilience/breaker.go](../../internal/platform/resilience/breaker.go) | 下游连续不健康时，暂停继续向它发送请求。 | 76 | +6 / −1 |
| [internal/platform/telemetry/doc.go](../../internal/platform/telemetry/doc.go) | 包说明、文件清单与阅读顺序 | 15 | +15 / −0 |
| [internal/platform/telemetry/kafka.go](../../internal/platform/telemetry/kafka.go) | 在 Kafka 消息头中传递调用链，消息正文仍只包含订单业务字段。 | 51 | +51 / −0 |
| [internal/platform/telemetry/metrics.go](../../internal/platform/telemetry/metrics.go) | 集中定义本阶段的指标名称、标签和注册方式。 | 90 | +90 / −0 |
| [internal/platform/telemetry/middleware.go](../../internal/platform/telemetry/middleware.go) | 在 HTTP 与 gRPC 的入口、出口测量耗时。 | 81 | +81 / −0 |
| [internal/platform/telemetry/telemetry_test.go](../../internal/platform/telemetry/telemetry_test.go) | 不用监控后端验证指标和跨 Kafka 的关联信息。 | 148 | +148 / −0 |
| [internal/platform/telemetry/tracing.go](../../internal/platform/telemetry/tracing.go) | 创建进程自己的调用链提供者，并把 span 批量送到 Jaeger。 | 80 | +80 / −0 |
| [internal/shop/app/inventoryservice/app.go](../../internal/shop/app/inventoryservice/app.go) | 库存进程的资源组装与退出流程。 | 119 | +15 / −4 |
| [internal/shop/app/orderservice/app.go](../../internal/shop/app/orderservice/app.go) | 装配订单服务的依赖、路由和退出流程。 | 173 | +26 / −73 |
| [internal/shop/app/orderservice/dependencies.go](../../internal/shop/app/orderservice/dependencies.go) | 准备订单进程需要的连接，并明确由谁关闭它们。 | 140 | +140 / −0 |
| [internal/shop/app/orderservice/doc.go](../../internal/shop/app/orderservice/doc.go) | 包说明、文件清单与阅读顺序 | 18 | +4 / −1 |
| [internal/shop/app/orderservice/metrics.go](../../internal/shop/app/orderservice/metrics.go) | 记录下单进入“结果待核对”的告警与计数。 | 40 | +6 / −8 |
| [internal/shop/app/orderservice/metrics_test.go](../../internal/shop/app/orderservice/metrics_test.go) | 检查待核对日志与指标的实际内容。 | 61 | +15 / −9 |
| [internal/shop/app/orderservice/observations.go](../../internal/shop/app/orderservice/observations.go) | 在订单服务的接口外面加计数，不把 Prometheus 类型传进业务流程。 | 83 | +83 / −0 |
| [internal/shop/app/orderservice/servers.go](../../internal/shop/app/orderservice/servers.go) | 用已经准备好的依赖装配业务，并创建两个 HTTP 入口。 | 72 | +72 / −0 |
| [internal/shop/app/orderworker/app.go](../../internal/shop/app/orderworker/app.go) | 启动订单消息消费者，并管理它的停机顺序。 | 135 | +14 / −4 |
| [internal/shop/events/consumer.go](../../internal/shop/events/consumer.go) | 逐条处理 Kafka 消息，并在数据库成功后提交读取位置。 | 150 | +52 / −19 |
| [internal/shop/events/consumer_test.go](../../internal/shop/events/consumer_test.go) | 证明消费者在数据库成功后才提交位点。 | 215 | +53 / −1 |
| [internal/shop/events/publisher.go](../../internal/shop/events/publisher.go) | 把订单创建消息发送到 Kafka。 | 81 | +23 / −7 |
| [internal/shop/inventoryclient/doc.go](../../internal/shop/inventoryclient/doc.go) | 包说明、文件清单与阅读顺序 | 13 | +1 / −0 |
| [internal/shop/inventoryclient/inventory_client.go](../../internal/shop/inventoryclient/inventory_client.go) | 给库存 gRPC 调用加上时间限制和故障保护。 | 184 | +8 / −0 |
| [internal/shop/inventoryclient/metrics.go](../../internal/shop/inventoryclient/metrics.go) | 从现有熔断器读取状态，并记录真正发送的查询重试。 | 43 | +43 / −0 |
| [internal/shop/order/service.go](../../internal/shop/order/service.go) | 按顺序完成下单，并在失败时判断能否安全重试。 | 208 | +7 / −1 |
| [internal/shop/order/service_test.go](../../internal/shop/order/service_test.go) | 逐个模拟下单步骤失败，核对库存补偿与幂等记录。 | 340 | +34 / −3 |
| [internal/shop/sales/repository.go](../../internal/shop/sales/repository.go) | 在一个 MySQL 事务里完成订单去重和销量累加。 | 86 | +9 / −8 |
| [internal/shop/sales/repository_integration_test.go](../../internal/shop/sales/repository_integration_test.go) | 在真实 MySQL 中验证销量去重与事务回滚。 | 152 | +22 / −6 |
| [internal/shop/sales/service.go](../../internal/shop/sales/service.go) | 在处理订单消息之前验证业务字段。 | 38 | +5 / −4 |
| [进度.md](../../进度.md) | OpsDoctor 进度看板 | 163 | +10 / −10 |
| [项目计划书.md](../../项目计划书.md) | OpsDoctor 项目计划书：Go 故障诊断 Agent + 可复现故障靶场 | 2021 | +10 / −3 |
<!-- PHASE_FILES_END -->

按改动包列出新增导出成员与关键函数。既有函数若接线变化较大也保留，便于逐行学习；普通不变成员不重复列出。

<!-- PHASE_SYMBOLS_START -->
#### cmd/alertsink

| 名称 | 位置 | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| `main` | [main.go:14](../../cmd/alertsink/main.go#L14) | main 等待清理完成后再退出，避免 os.Exit 跳过 HTTP 的关闭流程 | Go 进程入口 |

#### internal/loadgen

| 名称 | 位置 | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| `Run` | [app.go:42](../../internal/loadgen/app.go#L42) | Run 把启动准备排除在统计之外，实际完成数从工作者开始运行时累计 | cmd/loadgen.main |
| `*generator.execute` | [client.go:28](../../internal/loadgen/client.go#L28) | execute 把一次逻辑下单限制在两次 HTTP 尝试，第二次沿用同一个 job 的 key | generator.run 的工作者 |
| `generator` | [generator.go:56](../../internal/loadgen/generator.go#L56) | generator 把调度数据和发送依赖放在一起，随机源只允许 schedule 所在 goroutine 使用 | loadgen.Run |
| `newGenerator` | [generator.go:71](../../internal/loadgen/generator.go#L71) | newGenerator 用 v=100 分散最热的一小组商品，仍让它们远比长尾商品常见 | loadgen.Run、测试 |

#### internal/platform/adminserver

| 名称 | 位置 | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| `New` | [server.go:35](../../internal/platform/adminserver/server.go#L35) | New 创建未启动的 HTTP 服务器 | 对应 app 或入口 |

#### internal/platform/alertsink

| 名称 | 位置 | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| `Run` | [app.go:33](../../internal/platform/alertsink/app.go#L33) | Run 先验证令牌再开放端口，漏配环境变量时不能意外启动一个无认证接收器 | cmd/alertsink.main |
| `serve` | [app.go:58](../../internal/platform/alertsink/app.go#L58) | serve 的结果通道只有一次写入，主流程退出前一定读到它，不遗留监听 goroutine | 同包 Run |
| `notification` | [handler.go:28](../../internal/platform/alertsink/handler.go#L28) | notification 不保存注解正文，验收只需要确定哪个服务的哪个症状已经触发或恢复 | alertsink.Handler |
| `Handler` | [handler.go:38](../../internal/platform/alertsink/handler.go#L38) | Handler 由 app 注入令牌和日志器，测试可以直接发送内存请求，不需要启动 Alertmanager | alertsink.Run、认证测试 |
| `safeLabel` | [handler.go:68](../../internal/platform/alertsink/handler.go#L68) | safeLabel 控制日志中每个外部字段的长度，错误通知不能无限撑大一条日志 | Handler |

#### internal/platform/logging

| 名称 | 位置 | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| `FromContext` | [logging.go:47](../../internal/platform/logging/logging.go#L47) | FromContext 供日志包装层取出关联信息 | HTTP/gRPC 日志、订单、消费者 |

#### internal/platform/mysqlx

| 名称 | 位置 | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| `Config` | [mysql.go:34](../../internal/platform/mysqlx/mysql.go#L34) | Config 描述一个服务独占的连接池，实际环境变量在服务 app 包读取 | 对应 app 的配置读取 |
| `Open` | [mysql.go:44](../../internal/platform/mysqlx/mysql.go#L44) | Open 创建连接池并 Ping 验证可达性 | 三个业务 app、shopctl 和集成测试 |

#### internal/platform/resilience

| 名称 | 位置 | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| `*Breaker.State` | [breaker.go:71](../../internal/platform/resilience/breaker.go#L71) | State 读取底层状态，测试用它检查恢复过程，而不是猜测内部计数 | 熔断状态 GaugeFunc |

#### internal/platform/telemetry

| 名称 | 位置 | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| `KafkaHeaders` | [kafka.go:21](../../internal/platform/telemetry/kafka.go#L21) | KafkaHeaders 借用当前消息的切片，只在发送前修改，不能与写入 Kafka 的动作并发 | Kafka Publisher / Consumer |
| `*KafkaHeaders.Get` | [kafka.go:24](../../internal/platform/telemetry/kafka.go#L24) | Get 按 HTTP/W3C 头的习惯忽略大小写，不要求发送端恰好使用小写 | 传播器 Extract |
| `*KafkaHeaders.Set` | [kafka.go:34](../../internal/platform/telemetry/kafka.go#L34) | Set 分配自己的切片，既替换重复键，也不修改别人还持有的旧消息头 | 传播器 Inject |
| `*KafkaHeaders.Keys` | [kafka.go:45](../../internal/platform/telemetry/kafka.go#L45) | Keys 只暴露字段名，传播器通过 Get 读取对应值 | 传播器与测试 |
| `Metrics` | [metrics.go:26](../../internal/platform/telemetry/metrics.go#L26) | Metrics 保存本进程的仪表，具体在哪里加一次，由调用它的业务适配器决定 | 各 app 与观测包装 |
| `NewMetrics` | [metrics.go:44](../../internal/platform/telemetry/metrics.go#L44) | NewMetrics 的名称都由本包固定，重复注册说明代码有错，所以用 MustRegister 尽早暴露 | telemetry.New、loadgen、alertsink |
| `*Metrics.Counter` | [metrics.go:68](../../internal/platform/telemetry/metrics.go#L68) | Counter 的标签名由代码固定；调用处还要保证标签值来自有限的分类，不能塞入用户输入 | NewMetrics |
| `*Metrics.Histogram` | [metrics.go:75](../../internal/platform/telemetry/metrics.go#L75) | Histogram 用从毫秒到十秒的桶覆盖正常请求和超时，特别保留告警门槛 1 秒 | NewMetrics |
| `*Metrics.Register` | [metrics.go:83](../../internal/platform/telemetry/metrics.go#L83) | Register 允许 app 登记熔断状态等按需读取的采集器，仍统一添加服务标签 | Database、inventoryclient.RegisterMetrics |
| `*Metrics.Database` | [metrics.go:88](../../internal/platform/telemetry/metrics.go#L88) | Database 读取 database/sql 已有的池统计，不另外发 SQL，也不会占用一个数据库连接 | 三个业务 app |
| `*Metrics.HTTPMiddleware` | [middleware.go:29](../../internal/platform/telemetry/middleware.go#L29) | HTTPMiddleware 放在恢复和限流之前，这样被拒绝的请求也能进入总请求数 | orderservice.routes |
| `*Metrics.Server` | [middleware.go:53](../../internal/platform/telemetry/middleware.go#L53) | Server 包在恢复拦截器外层，panic 转成 Internal 后也会被记录 | inventoryservice.serve |
| `*Metrics.Client` | [middleware.go:63](../../internal/platform/telemetry/middleware.go#L63) | Client 的 target 使用 app 提供的服务名称，不能直接使用包含任意地址的请求参数 | orderservice.openInventory |
| `rpcCode` | [middleware.go:73](../../internal/platform/telemetry/middleware.go#L73) | rpcCode 避免本地 deadline 被笼统地记为 Unknown，方便区分超时与服务端故障 | Server、Client |
| `Runtime` | [tracing.go:36](../../internal/platform/telemetry/tracing.go#L36) | Runtime 不写 OTel 全局变量，插件都显式接收 Provider，测试之间因此不会串到另一套调用链 | 三个业务 app、Kafka 适配器 |
| `New` | [tracing.go:44](../../internal/platform/telemetry/tracing.go#L44) | New 的采样方式交给 SDK 读取 OTEL_TRACES_SAMPLER 与 OTEL_TRACES_SAMPLER_ARG | 对应 app 或入口 |
| `*Runtime.Shutdown` | [tracing.go:68](../../internal/platform/telemetry/tracing.go#L68) | Shutdown 在业务排空后执行，给最后一批 span 自己的发送预算 | 三个业务 app 的 defer |
| `Finish` | [tracing.go:75](../../internal/platform/telemetry/tracing.go#L75) | Finish 只记录固定的失败描述，原始错误可能含业务值，不能整段复制进追踪后端 | Kafka Publish / process |

#### internal/shop/app/inventoryservice

| 名称 | 位置 | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| `Run` | [app.go:37](../../internal/shop/app/inventoryservice/app.go#L37) | Run 拥有库存进程的完整生命周期 | 对应 cmd/main.go |
| `serve` | [app.go:78](../../internal/shop/app/inventoryservice/app.go#L78) | serve 在两个端口都绑定成功后才接收请求 | 同包 Run |

#### internal/shop/app/orderservice

| 名称 | 位置 | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| `routes` | [app.go:110](../../internal/shop/app/orderservice/app.go#L110) | routes 最外层建立 span 并计时，连被限流拒绝的请求也能被观测到 | servers |
| `Run` | [app.go:54](../../internal/shop/app/orderservice/app.go#L54) | Run 的 result 是具名返回值，defer 可以把关闭连接的错误一并交给入口 | 对应 cmd/main.go |
| `connectInventory` | [app.go:91](../../internal/shop/app/orderservice/app.go#L91) | connectInventory 把首次 DNS 查询和握手放进启动预算，避免第一位用户替服务承担建连等待 | openInventory、连接测试 |
| `openInventory` | [dependencies.go:107](../../internal/shop/app/orderservice/dependencies.go#L107) | openInventory 把插件放到每次真实 RPC 外层，重试发生几次就记录几次调用 | openDependencies |
| `*dependencies.close` | [dependencies.go:125](../../internal/shop/app/orderservice/dependencies.go#L125) | close 既用于启动失败，也用于正常停机，所以只关闭实际打开过的资源 | Run、启动失败的 defer |
| `dependencies` | [dependencies.go:37](../../internal/shop/app/orderservice/dependencies.go#L37) | dependencies 只保存进程拥有的连接 | openDependencies、servers |
| `openDependencies` | [dependencies.go:46](../../internal/shop/app/orderservice/dependencies.go#L46) | openDependencies 失败时自己清理；成功后把关闭责任交给 Run，避免半途失败留下连接 | orderservice.Run |
| `openRedis` | [dependencies.go:88](../../internal/shop/app/orderservice/dependencies.go#L88) | openRedis 不记录命令参数，因为幂等键和缓存正文可能包含请求数据 | openDependencies |
| `reviewRecorder` | [metrics.go:24](../../internal/shop/app/orderservice/metrics.go#L24) | reviewRecorder 把日志与计数放在同一个动作里，避免某个失败分支只做其中一项 | services、Create 的待核对分支 |
| `newReviewRecorder` | [metrics.go:30](../../internal/shop/app/orderservice/metrics.go#L30) | newReviewRecorder 使用 app 自己的注册表，不污染全局注册表，也便于测试重复创建服务 | services |
| `*reviewRecorder.RecordReview` | [metrics.go:37](../../internal/shop/app/orderservice/metrics.go#L37) | RecordReview 使用固定原因分类，不把原始连接错误、DSN 或请求体写入告警 | order.Service.unknown |
| `observedCache` | [observations.go:25](../../internal/shop/app/orderservice/observations.go#L25) | observedCache 统计每次缓存读取，第二次检查缓存也会计数，因此它不等于 HTTP 请求数 | services |
| `observedCache.Get` | [observations.go:31](../../internal/shop/app/orderservice/observations.go#L31) | Get 把读取失败单独记为 error，不让 Redis 故障看起来只是普通未命中 | product.Service.Get |
| `observedProducts` | [observations.go:47](../../internal/shop/app/orderservice/observations.go#L47) | observedProducts 放在 singleflight 后面的仓储接口上，合并后的查询只计一次回源 | services |
| `observedProducts.Get` | [observations.go:53](../../internal/shop/app/orderservice/observations.go#L53) | Get 在真正查库前计数，查询失败也算给数据库带来过一次请求 | product.Service 回源 |
| `observedIdempotency` | [observations.go:59](../../internal/shop/app/orderservice/observations.go#L59) | observedIdempotency 不改变占位逻辑，其余完成、放弃和待核对动作仍由原实现处理 | services |
| `observedIdempotency.Reserve` | [observations.go:65](../../internal/shop/app/orderservice/observations.go#L65) | Reserve 只统计成功读到的确定状态 | order.Service.Create |
| `services` | [servers.go:27](../../internal/shop/app/orderservice/servers.go#L27) | services 在构造函数之间传递接口，让商品、订单包不用自己打开连接或注册全局指标 | servers |
| `servers` | [servers.go:50](../../internal/shop/app/orderservice/servers.go#L50) | servers 只创建对象，不开始监听 | Run |

#### internal/shop/app/orderworker

| 名称 | 位置 | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| `Run` | [app.go:43](../../internal/shop/app/orderworker/app.go#L43) | Run 先停消费者，再关闭 Kafka 和数据库，不能在当前事务尚未完成时拆掉连接 | 对应 cmd/main.go |

#### internal/shop/events

| 名称 | 位置 | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| `decodeMessage` | [consumer.go:132](../../internal/shop/events/consumer.go#L132) | decodeMessage 在开始事务前检查正文和 key，坏消息不能被错记为已消费 | Consumer.process |
| `MessageReader` | [consumer.go:36](../../internal/shop/events/consumer.go#L36) | MessageReader 是消费者需要的最小能力，真实实现是 kafka.Reader | Consumer |
| `Consumer` | [consumer.go:48](../../internal/shop/events/consumer.go#L48) | Consumer 不拥有 reader 的关闭权，app 等 Run 返回后统一关闭连接 | orderworker.Run |
| `NewReader` | [consumer.go:57](../../internal/shop/events/consumer.go#L57) | NewReader 从消费组已提交的位置继续读取，新组则从最早保留的消息开始 | orderworker.Run |
| `NewConsumer` | [consumer.go:64](../../internal/shop/events/consumer.go#L64) | NewConsumer 要求有限的处理预算，停止服务时不能无限等一条消息 | orderworker.Run、单元测试 |
| `*Consumer.Run` | [consumer.go:72](../../internal/shop/events/consumer.go#L72) | Run 串行处理消息 | orderworker.serve |
| `*Consumer.process` | [consumer.go:98](../../internal/shop/events/consumer.go#L98) | process 把解析、处理、提交写在同一处，读者可以直接核对先后顺序 | Consumer.Run、单元测试 |
| `Publisher` | [publisher.go:28](../../internal/shop/events/publisher.go#L28) | Publisher 使用 kafka.Writer 自带的连接复用，不为每个订单重新连接 broker | orderservice.dependencies |
| `NewPublisher` | [publisher.go:43](../../internal/shop/events/publisher.go#L43) | NewPublisher 禁用自动建主题 | openDependencies |
| `*Publisher.Publish` | [publisher.go:52](../../internal/shop/events/publisher.go#L52) | Publish 收到错误时不声称发送成功 | order.Service.Create |
| `*Publisher.Close` | [publisher.go:77](../../internal/shop/events/publisher.go#L77) | Close 等发送中的调用结束后再执行，避免关闭过程与新的 Publish 争抢资源 | dependencies.close |

#### internal/shop/inventoryclient

| 名称 | 位置 | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| `Client` | [inventory_client.go:41](../../internal/shop/inventoryclient/inventory_client.go#L41) | Client 共用一条 gRPC 连接 | orderservice.services |
| `*Client.GetStock` | [inventory_client.go:87](../../internal/shop/inventoryclient/inventory_client.go#L87) | GetStock 的所有尝试共享一个截止时间，不能每次重试都重新获得完整预算 | 商品查询与 shopctl |
| `*Client.RegisterMetrics` | [metrics.go:21](../../internal/shop/inventoryclient/metrics.go#L21) | RegisterMetrics 必须在客户端开始使用之前调用，之后 retryCounter 只负责线程安全地加数 | openDependencies |
| `breakerValue` | [metrics.go:34](../../internal/shop/inventoryclient/metrics.go#L34) | breakerValue 显式列出三种状态，读者可以直接与告警表达式对照 | GaugeFunc 回调 |

#### internal/shop/order

| 名称 | 位置 | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| `*Service.Create` | [service.go:76](../../internal/shop/order/service.go#L76) | Create 遇到结果不明就停止自动处理，不能把“没有收到成功答复”当成“什么都没发生” | 订单 HTTP handler |

#### internal/shop/sales

| 名称 | 位置 | 作用 | 被谁调用 / 使用 |
|---|---|---|---|
| `*MySQLRepository.Apply` | [repository.go:47](../../internal/shop/sales/repository.go#L47) | Apply 返回的 duplicate 表示已经记过这笔订单，消费者因此能区分“新增”和“重复” | sales.Service.Handle |
| `Repository` | [service.go:22](../../internal/shop/sales/service.go#L22) | Repository 的 Apply 必须把去重与加销量作为一件事完成，不能只成功一半 | sales.Service 的构造参数 |
| `*Service.Handle` | [service.go:33](../../internal/shop/sales/service.go#L33) | Handle 的 error 为 nil 时，消费者才可以提交位点；bool 用来单独统计重复消息 | Consumer.process |

<!-- PHASE_SYMBOLS_END -->

### 数据库变更

没有新增业务表、索引或迁移。`004_metrics_user.sh` 新建 metrics 账号，限制最多三个连接；授权 PROCESS、REPLICATION CLIENT、performance_schema 与 shop 的 SELECT。没有写数据或改结构权限。旧卷手动运行同一脚本，不重建库。

### 接口变更

业务 HTTP、protobuf、Kafka JSON 契约不变。三个业务管理端口 6061/6062/6063 增加标准 /metrics、/debug/pprof；loadgen 的 6070 增加 /metrics。新增 alertsink 8091 的 POST /api/v1/alerts，要求 Bearer 令牌，正文最多 1 MiB；成功 204，认证失败 401，非法正文 400。

监控发布端口为 Grafana 3000、Prometheus 9090、Jaeger 16686/4317、Loki 3100、Alertmanager 9093。全部绑定 127.0.0.1；三个 exporter 和 Alloy 不发布宿主机端口。

### 配置变更

| 环境变量 | 含义 | 默认值 |
|---|---|---|
| APP_VERSION | Compose 构建写入的版本 | phase-03；直接 go run 为 dev |
| GRAFANA_ADMIN_PASSWORD | 首次管理员密码 | 无，必须提供；示例只有演示值 |
| MYSQL_EXPORTER_PASSWORD | metrics 账号密码 | 无，必须提供 |
| ALERTSINK_ADDR | 临时接收器监听地址 | :8091 |
| ALERT_WEBHOOK_TOKEN | 接收与推送共用凭据，已有契约开始接线 | 无，至少 32 字符 |
| OTEL_SERVICE_NAME | 服务身份，已有契约开始接线 | 各 app 的固定名称 |
| OTEL_EXPORTER_OTLP_ENDPOINT | OTLP gRPC 地址，已有契约开始接线 | 宿主机程序空值不导出；Compose 为 http://jaeger:4317 |
| OTEL_TRACES_SAMPLER | 采样方式 | Compose 为 parentbased_traceidratio |
| OTEL_TRACES_SAMPLER_ARG | 根请求采样比例 | Compose 为 0.1 |

配置与第 5.8.4 节、`.env.example` 同步。密钥只从环境读取，本机 `.env` 不提交。

### 部署变更

九个监控产品加 alertsink，共十个新容器；每个都设置 mem_limit，所有十七个容器使用轮换 JSON 日志。Prometheus 保留七天并限制 1 GB，Loki 48 小时，Jaeger 内存最多五千条。Grafana 自动 provision 三数据源与十二面板；Alloy 只收本项目五个 Go 进程日志。

新增目录配置入口见 [deploy/README](../../deploy/README.md)。CI 增加监控接线冒烟，运行器仍固定 ubuntu-24.04；任务上限从 20 分钟增为 30 分钟，容纳十个新增容器的下载和启动。

## 四、测试

| 新增测试 | 文件 | 验证场景 |
|---|---|---|
| TestPublishSurvivesCancellation | order/service_test.go | 提交后取消原 ctx，发送仍可执行且保留值、有限 deadline |
| TestKafkaPropagation | telemetry/telemetry_test.go | trace 与远端父 span、旧头替换、无效头忽略 |
| TestHTTPMetrics | 同上 | 模板路由、未知路径、429、panic 恢复后计数 |
| TestRPCMetrics | 同上 | 成功、业务拒绝、本地超时，客户端与服务端均有样本 |
| TestCurrentSpanOverridesStoredFields | logging/logging_test.go | 日志使用子 span，保留 request_id |
| TestObservabilityEndpoints | adminserver/server_test.go | 独立指标注册表与 goroutine profile 可读取 |
| TestConsumerObservations | events/consumer_test.go | 消费 span 和 processed/duplicate/failed 分类 |
| TestAlertAuthentication | alertsink/handler_test.go | 缺令牌、错令牌、合法请求、非法 JSON、尾随内容、超大正文 |

既有 TestIntegrationDuplicateSales 加强为同时检查 19 次重复返回与销量只加一次。其他改动后的测试保留原业务断言。

本地运行：

```text
gofmt -l cmd gen internal tools
go mod tidy
go vet ./...
staticcheck ./...
go run ./tools/commentcheck ./...
go run ./tools/archcheck ./...
go test -race ./... -count=1 -timeout=180s -json
```

全部退出 0；格式无输出，commentcheck 覆盖 147 个非生成 Go 文件。无外部依赖轮为 75 个顶层测试通过、16 个集成测试明确跳过、25 个有测试包通过。Windows 使用可见的 gcc 和 CGO_ENABLED=1。

按 README 加载六个专用 TEST DSN 与 TEST_REDIS_ADDR 后，单独运行 16 个 Integration 测试全部通过。只触碰白名单测试库与 Redis DB 14/15 的测试键。旧 `.env` 缺两项 DSN 时采用明确构造和校验库名，没有用业务库替代。

CI YAML 和三个内嵌 Python 脚本语法通过。远端 CI 尚未运行，不能作为已通过证据。

## 五、工程验收结果

| 计划验收项 | 当前结论 | 真实证据 |
|---|---|---|
| Jaeger 完整下单调用链 | 通过直接 Jaeger 查询 | trace de18cb3a9dd94612b021dc9c4ff56854，29 span、三个服务、八个环节完整，父子关系核对 |
| Grafana 按 trace_id 查全条日志 | 通过 | 通过 Grafana Loki 数据源代理得到三个服务的三条关联日志 |
| 看板面板都有数据 | 通过 | 从 Grafana 读取看板后逐个查询十二面板，都有非空数据，无错误 |
| 停 worker 后三分钟内收到消费延迟告警 | 通过 | 目标 200 QPS，167.064 秒收到 ConsumerLagHigh；恢复后收到 resolved |
| 完整监控栈接线 | 待完成 | Grafana 的 Jaeger 数据源对 2.22.0 返回 404，等待兼容版本决定 |

以上运行命令、时间、原始数值和限制见 [notes 阶段 3](../notes.md#phase-03)。最终暂停流量后 lag=0，`ordered_quantity=2066809 sold_quantity=2066809 gap=0`。

本阶段台账条目为 [P3-1 至 P3-10](../数据台账.md#阶段-3可观测性与告警验收进行中)，包含成功结果、混合实验错误和版本兼容失败。

全栈内存快照：17 个容器实际合计 **1938.2 MiB，约 1.89 GiB**，配置上限合计 5056 MiB。采样时目标流量 200 QPS，Docker 可见 16 逻辑 CPU、约 15.18 GiB 内存。MySQL 占 680.5/768 MiB，Grafana 183.9/256 MiB；其余逐容器值见 notes。这个快照说明本次能运行，不是峰值上界，也不包括 Windows 与 Docker 虚拟机的全部开销。

这轮混合实验累计有 98 错误、57 dropped，包含暂停与故障过程，不能当成新的性能基准。简历可引用阶段 2 的实际五分钟结果并说明环境，也可引用本阶段完整链路、三类数据关联和积压发现实验；不能写未实现的自动诊断准确率或修复收益。

## 六、偏离计划之处

当前没有擅自改变技术栈。最新 Jaeger 与最新 Grafana 的接口不兼容，按计划书第 2.2 节已请求固定 Jaeger 2.20.0 的例外许可，尚未收到确认，未改版本。[决定记录](../decisions.md)记录当前版本和原因；批准后再同步计划与记录。

订单发送上下文修复由用户明确要求。沿用用户对 needs_review 的要求，将其补入第 5.8.5 节标签表并并入统一注册表。新增缓存 error 分类，避免把 Redis 故障伪装成 miss。

Loki 官方镜像缺少 wget，所以不保留无法执行的容器内健康命令，改由宿主机和 CI 查 /ready。没有引入额外镜像或系统软件。

## 七、已知问题与遗留事项

- Grafana / Jaeger 兼容问题仍阻塞最终验收和推送收尾。
- Jaeger 内存数据在重启后丢失，默认只采样十分之一根请求；Loki、Prometheus 也有保留窗口。这是本机实验配置。
- 客户端取消修复不等于可靠消息。订单提交后进程崩溃或 Kafka 故障仍可能少记销量；本阶段不做 Outbox。
- 根治扣库存超时的不确定性，需要 Deduct 自身携带请求 ID 去重；本阶段不实现。待核对结果仍需人工处理。
- Alloy 的 Docker socket 挂载有较大权限，`:ro` 不是 Docker API 权限隔离。后续医生绝不获得该挂载，按第 5.5.9 节控制权限。
- 消费积压告警的发现时间取决于流量、阈值和持续窗口；167.064 秒是本次实验，不是任何负载的保证。
- 阶段 4 的故障注入和阶段 5 以后的医生能力尚未实现。

## 八、注释自查结论

commentcheck 通过，三层说明已检查。新增分组目录包含 README；新包 doc.go 列出全部 Go 文件和测试，代码地图由 -map 更新。每个新普通 Go 文件有“本文件、主要内容、阅读建议”，函数说明侧重为什么这样安排。

检查中修复过 doc.go 漏文件、较长函数缺内部解释的问题。还改写了“请求自己的 ctx 控制发送”等已经不准确的旧句子，说明现在使用独立收尾预算；销量接口清楚解释 bool 和 error 的区别。订单入口拆成有名字的步骤，减少同一行堆放插件选项。

Kafka 传播、模板标签、日志当前 span、采样、GaugeFunc 读取时机、令牌摘要比较和连接关闭顺序均有针对性解释。测试表格说明各分支想排除什么问题。材料用最小例子解释切片指针接收者、接口嵌入、闭包采集与 WithoutCancel，不用固定套话替代讲解。

## 九、学习指引

阅读[学习材料](../learn/phase-03-observability.md)，先弄清指标、调用链、日志各回答什么，再从 cmd 进入 app 和 telemetry。建议用约十小时分三轮：两小时跑环境与看数据，五小时按文件追一次下单，三小时做实验并回答问题。学习验收由用户勾选。

## 十、下一阶段预告

阶段 4 是故障注入器与场景库，会让故障能够按配置复现。本阶段没有开始它。等阶段 3 完整收尾、用户学习并明确授权后才继续；目前无需申请付费大模型密钥。
