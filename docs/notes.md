# 实测记录

<a id="phase-03"></a>

## 2026-10-23：阶段 3 开发记录

首次监控栈启动遇到两个实际问题：Loki 3.7.9 精简镜像没有 wget，容器里的服务可运行，但复制来的健康探测报 executable not found，导致 Alloy 依赖等待失败。去掉无法执行的镜像内探测，改由宿主机与 CI 访问 /ready，Alloy 使用自身重试等待 Loki。另一个是 Jaeger 2.22.0 已移除 /api/services；直接请求为 404，Grafana 的 Jaeger 数据源健康 API 也实测为 400 + request failed: 404。已查官方发布说明和未关闭问题 #87，按计划书第 2.2 节询问是否例外固定兼容的 2.20.0，尚未擅自改版本。

用户确认阶段 2 后，检查干净工作区，拉取远端，将阶段 2 合入 main 并推送。main 已有的权限边界和阶段 9 OpenSearch 计划更新一并保留；新分支为 phase-03-observability。完整读完合并后的 1986 行计划书。

先修复订单提交后的消息发送取消问题：用 WithoutCancel 保留值，另加 cleanupTimeout。测试在 repository.Create 返回时取消原请求，检查 Publish 仍可执行、含有限 deadline、保留上下文值。相关订单回归通过。

随后接入统一注册表、显式 Provider、HTTP/gRPC/SQL/Redis 追踪和 Kafka 传播。第一轮编译提示需要 go mod tidy，整理模块后通过。首次 commentcheck 指出 doc.go 缺新文件、telemetry.NewMetrics 缺内部讲解，逐项补齐。全仓库无外部依赖的普通测试通过，最终 race 与真实依赖验收另记。

监控镜像版本从官方稳定发布查询并固定，九个镜像已全部拉取成功。首次 Compose 启动前只补了本机 .env 缺少的随机凭据，没有打印或覆盖已有值。保留已有 MySQL、Redis、Kafka 数据卷，loadgen 设置 QPS=0。旧 MySQL 卷不会再次运行 init，因此手动运行 004_metrics_user.sh 补建只读监控账号，执行成功。


### 管道与故障验收（07:29–07:52，America/Los_Angeles）

源码基线 `9e0d78c23b0e2f2f904ffb576c9aca7ed48d4707`，阶段 3 工作区；最终阶段提交会包含这些变更。默认采样 0.1，验证请求显式发送 W3C traceparent 的采样标志 01，保证这次实验可查。时间记录区分宿主机与容器；本次观察到两者约差 12 秒，告警耗时使用宿主机 monotonic 计时，不混减两台时钟。

- 真实下单：用户 880000303、商品 499997、数量 1，HTTP 201，订单 2038577，金额 1999 分。trace ID `de18cb3a9dd94612b021dc9c4ff56854`。
- `GET http://127.0.0.1:16686/api/traces/<trace-id>` 返回 29 个 span，属于 order-service、inventory-service、order-worker。覆盖 HTTP、Redis、gRPC 客户端/服务端、库存 SQL、订单 SQL、Kafka 发送/消费、销量 SQL。消费 span `ae1ce99c2028c91a` 的父节点为生产 span `f7caf9152ff30ab1`。库存服务端父节点等于订单侧 gRPC 客户端 span。
- 通过 Grafana `/api/datasources/proxy/uid/loki/loki/api/v1/query_range` 查询 `{service=~"order-service|inventory-service|order-worker"} | json | trace_id="de18cb3a9dd94612b021dc9c4ff56854"`，得到三个服务的三条日志。每条有该 trace ID、各自 span ID；启动日志不属于这条请求。
- 读取 Grafana `/api/dashboards/uid/shop-overview`，按返回的十二个面板配置逐个提交 `/api/ds/query`。十二个面板均无查询错误、含非空数据：QPS、P99、错误率、连接池、缓存命中率、消费积压、goroutine、进程内存、熔断、重试、连接池等待、幂等。
- Prometheus `/api/v1/targets` 八个目标全部 UP；另查 `mysql_up=1`、`redis_up=1`，证明 exporter 不只是端口能访问。
- 目标 200 QPS 时执行 `docker compose --env-file .env -f deploy/docker-compose.yml stop order-worker`。从发起停止到 alertsink 收到 `ConsumerLagHigh` 为 **167.064 秒**，小于三分钟。约第 31.754 秒首次观察到 lag=1183；告警要求 >1000 持续两分钟，还需评估和分组等待。
- 原始接收日志：容器时间 `2026-10-23T14:35:40.576248984Z`，level=INFO，status=firing，alertname=ConsumerLagHigh，alert_service=order-worker。恢复 worker 后 `14:36:40.572312443Z` 收到同名 resolved。没有把另一个 ServiceUnavailable 告警混作积压告警。
- 流量已暂停为 QPS=0。最终 lag=0，`go run ./cmd/shopctl check` 输出 `ordered_quantity=2066809 sold_quantity=2066809 gap=0`。本次没有删除数据卷、清除幂等记录、重新造数或补库存。

### 全栈内存快照（07:42:54）

命令 `docker stats --no-stream --format '{{json .}}'`，只汇总本项目的 17 个容器。采样时控制参数为 QPS=200、hot_sku_id=0。Docker 看到 16 个逻辑 CPU、16294768640 字节内存（约 15.18 GiB）。**实际容器内存合计 1938.2 MiB，约 1.89 GiB**；配置上限合计 5056 MiB，约 4.94 GiB。快照不是峰值，也不包括整个 Docker 虚拟机、Windows、镜像缓存的全部内存。MySQL 当时为 680.5/768 MiB，不能仅按总空闲量断言每个容器都有同样余量。

| 容器 | 实际内存 | 上限 |
|---|---:|---:|
| alertmanager | 13.19MiB | 64MiB |
| alertsink | 14.85MiB | 64MiB |
| alloy | 70.71MiB | 256MiB |
| grafana | 183.9MiB | 256MiB |
| inventory-service | 14.98MiB | 128MiB |
| jaeger | 196.5MiB | 512MiB |
| kafka | 420.2MiB | 1GiB |
| kafka-exporter | 13.8MiB | 64MiB |
| loadgen | 16.68MiB | 128MiB |
| loki | 166.1MiB | 512MiB |
| mysqld-exporter | 10.09MiB | 64MiB |
| order-service | 25.18MiB | 256MiB |
| order-worker | 14.53MiB | 128MiB |
| prometheus | 41.59MiB | 512MiB |
| redis | 44.1MiB | 256MiB |
| redis-exporter | 11.3MiB | 64MiB |
| shop-mysql | 680.5MiB | 768MiB |

这次流量用于填充看板与故障验收，不是新的无故障性能基准。压测器累计记录包含开始前暂停时间和实验过程：1144.235817297 秒、156073 次尝试、98 错误、57 dropped、累计平均 136.39933101262935 QPS、P50=3ms、P99=30ms。不能拿目标 200 替换这个实际累计均值，也不能把混合实验的 P99 与阶段 2 基准直接作性能提升比较。阶段 2 五分钟结果仍保留在原章节。

### 代码与配置回归

整理清单时发现本地 main 已新增 171f2ed（后续多系统计划）与 e6139da（数据台账规范）。先以 482aa22 保存阶段 3 实现，再把 main 合入本阶段分支。环境变量表一处冲突保留两代约定：阶段 3 的 ALERT_WEBHOOK_TOKEN 继续用于临时接收器，阶段 5 才启用 SHOP_ALERT_TOKEN 和 TARGETS_DIR。收尾流程改为九步，新增本阶段台账 P3-1 至 P3-10。未修改或推送 main。

订单 app 接入插件后变长，按计划拆成 dependencies.go 的连接准备与 servers.go 的服务装配。原有的打开顺序、失败清理、排空后关闭依赖顺序保留；重新构建订单和库存容器，均 healthy。新增测试验证发布时请求取消仍保留值与有效预算、子 span 覆盖旧日志关联、管理端点、消费分类、Kafka 传播、HTTP/RPC 指标和告警令牌校验。

最终一轮无外部依赖检查：gofmt 无输出，go vet、staticcheck、commentcheck、archcheck 均退出 0。commentcheck=147 个非生成 Go 文件。`go test -race ./... -count=1 -timeout=180s -json`：75 个顶层测试通过，16 个集成测试因未注入环境明确跳过，25 个有测试包通过。

单独注入六个专用测试库与 TEST_REDIS_ADDR 后，16 个 Integration 测试全部通过，包括 20 次相同消息中 19 次被识别为重复。本机旧 .env 缺 ORDER/SALES 两个 DSN；首次脚本在读缺少的键时退出。随后回退替换写法不能匹配不带查询参数的 DSN，被自动审批拒绝，测试没有执行。修正为从已有 TEST_MYSQL_DSN 分离库名、明确写入新库名，并逐个校验后才执行。没有对 shop 执行测试清理。

文档校验：四个最小 Go 示例均编译运行成功；十四段 PowerShell 示例只做语法解析，没有执行其中的故障实验。初版链接检查误把 AGENTS 的行内 Markdown 示例当成真实链接，忽略代码区域后，实际文件链接全部有效。Compose 十七个服务的固定镜像、内存上限、回环端口检查通过；容器内 promtool 检查采集配置与五条规则成功。

CI 已增加八个采集目标、exporter 后端连接、Loki 就绪、三数据源健康、看板、跨服务 trace 与日志的冒烟。YAML 及三个内嵌 Python 段语法通过；尚未推送，不能声称远端 CI 通过。Grafana/Jaeger 兼容问题待用户决定，工程验收尚未全部完成。

### 兼容例外批准后的对照与恢复（08:44–08:53）

用户批准仅固定 Jaeger 2.20.0，其他组件仍按最新稳定版规则。先保存 2.22.0 的原始响应，再只替换 Jaeger 镜像，数据源和 Jaeger 配置未改。Grafana 数据源配置 SHA-256 两次同为 `110e07eb43712c494bd9e60b30ec2a2a9bf692082b8039e5ba7e5b7d85f212e2`。直接 `/api/services` 和 Grafana 代理同路径由 404 变为 200，Grafana `/api/datasources/uid/jaeger/health` 由 400/ERROR 变为 200/OK。详细路径和上游链接在 [兼容决定](decisions.md#jaeger-compatibility)。

第一次新下单失败：用户 880000304、商品 499994，约 830ms 后返回 409，日志 reason=deduct_outcome_unknown，order_id=0，trace `e23cbbae6cc045f68343dca288810b7c`。只读 SQL 未找到该用户订单，库存仍为 100；幂等键 `phase03-final-ee53d76496c84f6dbb5e7ade877eb468` 原样保留，没有删除或重做。后续三次只读商品查询恢复正常，分别约 56.838/8.520/5.407ms。现有证据不足以确定这次库存调用超时的根因，不把它归因于 Jaeger 或声称已根治。

随后用独立用户 880000305 和商品 499993 新建另一笔验收订单，返回 201，订单 2066809。但 trace `a22b789638174b639fa2b1979e3c5794` 只有订单侧 span。库存和消费者日志带同一个 trace ID，证明关联传播到了服务；两者在容器时间 15:48:28、15:48:26 分别报告 `traces export: processor export timeout`。这批记录没送达，等待后也不能恢复。本地 CI 同口径脚本因此确实失败，未把它当成通过。

连接恢复后的独立订单：用户 880000306、商品 499992、数量 1，订单 **2066810**，HTTP 201，金额 1999。trace **2cd8e6e7f7ab413eb88850d3d0c18ede** 有 **29 个 span、三个服务**。库存服务端 span `3d7f82c1597ec109` 的父节点是客户端 `668add961d6be982`；消费者 `dbb5939fa22618aa` 的父节点是生产者 `7193cf093011b752`。没有重做前面待核对的订单。

经 Grafana 代理的服务列表返回四项（含 Jaeger 自身）、order-service 操作列表十五项、过去一小时 trace 搜索十二项、该 ID 详情一项，均为 HTTP 200。Grafana `/api/ds/query` 实际返回一个可用 trace 数据帧。本地运行 CI 的观测脚本，八个目标、三数据源、十二面板定义、三服务 trace 与关联日志全部通过。十二面板数据查询的完整验证仍引用前面的原始记录，不把“面板定义有十二个”偷换成这一刻所有速率面板都非空。

最终流量仍暂停，`shopctl check` 为 `ordered_quantity=2066811 sold_quantity=2066811 gap=0`。08:53:39 的新内存快照为 **1850.944 MiB，约 1.81 GiB**，此时 Jaeger=2.20.0、目标 QPS=0。与先前 2.22.0、目标 200 QPS 的快照条件不同，不据此计算优化比例。MySQL 为 **758.3/768 MiB**，接近单容器上限；全栈总量不高也不能说明这个容器还有足够加压余量。

| 容器 | 实际内存 | 上限 |
|---|---:|---:|
| alertmanager | 13.2MiB | 64MiB |
| alertsink | 15.01MiB | 64MiB |
| alloy | 68.68MiB | 256MiB |
| grafana | 152.5MiB | 256MiB |
| inventory-service | 18.7MiB | 128MiB |
| jaeger | 15.26MiB | 512MiB |
| kafka | 497.5MiB | 1GiB |
| kafka-exporter | 14.6MiB | 64MiB |
| loadgen | 20.96MiB | 128MiB |
| loki | 120.9MiB | 512MiB |
| mysqld-exporter | 10.44MiB | 64MiB |
| order-service | 19.02MiB | 256MiB |
| order-worker | 23.71MiB | 128MiB |
| prometheus | 52.98MiB | 512MiB |
| redis | 39.27MiB | 256MiB |
| redis-exporter | 9.914MiB | 64MiB |
| shop-mysql | 758.3MiB | 768MiB |

批准后的最终检查再次通过：gofmt、go vet、staticcheck、commentcheck、archcheck、全仓库 race（75 个顶层测试通过、16 个真实依赖测试明确跳过）。专用依赖的 16 项通过记录见前面；源码没有变化，不为凑次数重跑相同集成测试。随后更新文件地图、汇报、学习材料和台账，准备推送并等待 Linux CI。

### 首次 Linux CI 与空库初始化修复（08:59–09:09）

[CI 39983025343](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/39983025343) 的源码为 `575f363`。格式、tidy、vet、staticcheck、规则检查和无依赖 race 通过；全新 Compose 的 MySQL 初始化退出 1，后面的真实依赖与管道验收全部跳过，不能计为通过。

本机先用独立临时 MySQL 容器和演示凭据复现，完全不挂载业务卷。Windows 绑定挂载让脚本带执行位，首次初始化成功；将副本设为 Linux checkout 的 0644 权限后，官方入口改用 source，稳定复现 `/usr/local/bin/docker-entrypoint.sh: line 341: MYSQL_ONETIME_PASSWORD: unbound variable`。原因是 004_metrics_user.sh 的 `set -u` 修改了父 shell，官方入口后续读取可选变量时退出。

修复为用圆括号包住初始化内容，在子 shell 里设置严格选项和 MYSQL_PWD。第三个独立空卷在相同 0644 权限下正常启动；随后再次 source 脚本，父 shell 的 nounset 和 MYSQL_PWD 均未改变。实际以 metrics 登录，SELECT 1、读取 performance_schema 成功，SHOW GRANTS 只有 PROCESS、REPLICATION CLIENT、shop 和 performance_schema 的 SELECT。CI 失败分支增加 MySQL 日志，便于以后定位首次初始化问题；等待修复提交的完整 CI 结果。

### 最终 Linux CI 39984585001

[运行链接](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/39984585001)，源码提交 `efc9dc08d5179e557bbec639fdda3ef736b762ca`。2026-10-23 09:10:40 创建，09:19:16 完成（America/Los_Angeles），结论 success。固定 ubuntu-24.04 / Go 1.27.1 的格式、tidy、vet、staticcheck、commentcheck、archcheck、无依赖 race、全新十七容器 Compose、真实 MySQL/Redis race、库存和订单消息冒烟全部通过。指标与 Grafana 验收也通过：八个采集目标及后端连接、三个数据源健康、Jaeger 列表和搜索、十二面板定义、三个业务服务的 trace 和关联日志。

CI 原始摘要：commentcheck=147；small 造数 100 商品、400 订单，首次总耗时 21ms，重跑 5ms 并跳过。四个 HTTP 接口通过，同键回放返回相同订单 401。十秒压测实际 9.999819562999999 秒、199 请求、0 错误、0 dropped、19.900359076109062 QPS、P50=3ms、P99=21ms；商品/订单/列表分别 147/36/16。最终 `ordered_quantity=438 sold_quantity=438 gap=0`。这是 CI 小规模冒烟，不替代本机全量五分钟实验。

初始化复现用的三个临时容器和各自匿名测试卷已清理；本机原业务卷未修改或删除，loadgen 仍暂停。最终非 Markdown 文件 181 个，按路径排序、CRLF 归一化为 LF 后逐文件 SHA-256，再对“路径 + TAB + 文件摘要 + LF”的拼接结果计算 SHA-256：`9ca4c8d8ae1614afba6f074777a31da00c13f2a547ba323bf79282f4d0eaaa1d`。随后只补 Markdown 的验收链接、状态和文件清单，源码指纹不变；文档提交标记 `[skip ci]`，不重复跑已通过的相同源码。

阶段 3 分支已推送，工程验收完成。没有合并 main，没有勾选任何学习验收，没有开始阶段 4。

### 阶段 3 返工：gRPC DNS TXT 等待（2026-10-23）

用户验收提供的新证据：流量暂停 34.5 分钟后恢复，约一秒内的库存调用全部超时，两个熔断器打开，产生 12 笔待核对订单。这是用户提供的现场观察，不冒充下面由 Astra 重跑的数据。返工基线为 `1a30b1b`，仍在 phase-03-observability，不合并 main。

先在 order-service 容器对比 DNS：`nslookup -type=A inventory-service` 为 0.186619 秒、成功；`nslookup -type=TXT _grpc_config.inventory-service` 为 5.175181 秒、超时。Jaeger 对应为 0.184398 秒和 5.179011 秒。时间由宿主机 monotonic 记录，包含 docker exec 的开销，所以不能称为纯 DNS 网络耗时。地址查询可用而额外 TXT 查询超时，两类请求必须分开判断。

读取固定版本 grpc-go v1.84.0 的本地模块源码，确认 lookupHost → lookupTXT → 返回解析结果的顺序，和默认 30 分钟空闲后关闭解析器的行为。另读 stream.go，waitForResolvedAddrs 位于创建尝试与 TagRPC 之前。OTLP exporter v1.47.0 在 client.go 中同样调用 grpc.NewClient。源码路径和公开链接见 [决定记录](decisions.md#grpc-dns-txt)。

对照实验先只加入 INVENTORY_GRPC_IDLE_TIMEOUT 配置，设置 10s，尚未禁用服务配置；镜像版本 phase-03-dns-before。MySQL 保持 768 MiB、buffer pool 256 MiB。Compose 的常驻 loadgen 暂停，独立验收脚本以目标 20 QPS 运行，固定每五个请求为四次查商品、一次下单。每笔订单使用独立幂等键且数量为一，失败不重试、不删占位。商品降级虽然 HTTP 为 200，也计入失败/降级；单独列出下单失败和待核对，不能只看 HTTP 错误率。

重启实验先预热并持续发送五秒，再执行 `docker compose --env-file .env -f deploy/docker-compose.yml restart inventory-service`，计时从发起命令开始；随后观测 25 秒。闲置实验先确认一次成功库存查询，然后无 RPC 等待 25 秒，恢复流量并观测 25 秒。恢复时间定义为“最后一个失败请求完成之后，第一次成功请求完成”的时刻，且末尾至少继续正常两秒。每轮只执行一次，不能据此推导容量或普遍可用性承诺。

修复前重启：故障窗口 500 次请求，18 次失败或降级，其中商品降级 13 次、下单失败 5 次、待核对 5 次；最后失败结束于 1.228447 秒，稳定恢复于 1.253498 秒。重启命令退出 0。本轮短暂重启仍可复用已解析的地址，因此不能预先把全部停机失败都算作 TXT 等待。

修复前闲置：500 次请求，228 次失败或降级，其中商品降级 197 次、下单失败 31 次、待核对 10 次；最后失败结束于 12.252487 秒，稳定恢复于 12.323761 秒。netstat 显示库存连接从 ESTABLISHED 变成 TIME_WAIT，确认空闲回收已经发生。失败 trace `aa757b687403442a99b36e361878ae46`、`0572ada26c5c4c57b9e9e34b8c5df83a`、`c65d0674a60d445bb25c38e4ce166b25` 都只有 POST /orders、set、get、eval 四个 span，没有 gRPC span。原幂等键与待核对状态保留。

随后在库存客户端、手动库存命令和 OTLP 导出器加入代码选项。新增内存回归让库存/OTLP 连接真正空闲回收，再检查重建解析器收到禁用服务配置标志，并验证 RPC 和 span 实际送达；配置测试覆盖默认、实验、关闭、负数、超界和错误单位。三包的定向 race 全部通过。暂不调整 MySQL，先完成同条件的修复后对照。

修复后重启（09:56）：同样 500 次请求，17 次失败或降级，13 次商品降级、4 次下单失败及待核对；最后失败完成于 1.215923 秒，稳定恢复于 1.260281 秒。与修复前几乎一致，不把随机调度差异说成改善。

修复后闲置（09:58）：同样等待 25 秒，确认库存连接从 ESTABLISHED 变为 TIME_WAIT；随后 500 次全部成功，首个请求 0.007593 秒完成，无新待核对。首笔订单 2080411 的 trace `8123835dfe654e0580d60bdf2b96f099` 有 24 个 span、order-service / inventory-service / order-worker 三个服务，Grafana Loki 也查到三个服务的关联日志。这笔缓存路径与原来的 29 span 样本不同，不用固定 span 总数判断完整性。

两种对照完成之后才提高 MySQL 容器上限为 1536 MiB，保持原 256 MiB buffer pool 与 max_connections=100；保留业务卷。随后将默认镜像版本和空闲时间恢复为 phase-03 / 30m，重新构建三个业务服务。

本轮全套本地检查通过：gofmt、vet、staticcheck、commentcheck（148）、archcheck、全仓库 race（78 个顶层通过、16 个明确跳过、25 个有测试包通过）。集成测试首次装配环境时发现本机缺 TEST_REDIS_ADDR，脚本在运行测试前退出；明确使用本机 127.0.0.1:6379 后，六个 DSN 逐个校验专用库名，再运行 16 个 Integration，全部通过。测试固定 Redis DB 14/15，没有清理业务数据。

MySQL 调整后内存观察（10:03–10:05）：目标 200 QPS，约每 30 秒采样一次，共四次。MySQL 实际占用依次为 622.6、635.9、650.4、656.5 MiB，均以 1.5 GiB 为上限。17 个容器合计依次为 1942.800、2014.490、2070.132、2069.870 MiB。最后一轮约 2.02 GiB，配置上限合计由 5056 增至 5824 MiB。这是四个快照，不是全程峰值。数据库刚重建进程，不能把比旧 758.3 MiB 低解释为提高上限能降低内存。

| 容器 | 最后一次有流量快照 |
|---|---:|
| order-service | 22.59MiB / 256MiB |
| order-worker | 15.58MiB / 128MiB |
| inventory-service | 16.62MiB / 128MiB |
| shop-mysql | 656.5MiB / 1.5GiB |
| jaeger | 95.93MiB / 512MiB |
| loki | 200.3MiB / 512MiB |
| loadgen | 18.04MiB / 128MiB |
| mysqld-exporter | 10.36MiB / 64MiB |
| kafka-exporter | 15.18MiB / 64MiB |
| redis-exporter | 10.04MiB / 64MiB |
| alloy | 112.7MiB / 256MiB |
| alertmanager | 21.92MiB / 64MiB |
| kafka | 587.2MiB / 1GiB |
| redis | 47.97MiB / 256MiB |
| grafana | 148.9MiB / 256MiB |
| prometheus | 74.91MiB / 512MiB |
| alertsink | 15.13MiB / 64MiB |

实际配置复核：容器 HostConfig.Memory=1610612736 字节，SQL 查得 innodb_buffer_pool_size=268435456、max_connections=100。未扩大 buffer pool，也没有清理原数据。正常订单服务启动日志的 inventory_grpc_idle_timeout 为 30m0s。

这轮累计增量为 122.194203 秒、24325 次请求、267 错误、116 dropped，实际 199.068363 QPS；商品/订单/列表分别为 16967/4968/2390。库存服务有 267 次 FailedPrecondition，源码只把库存不足映射成此状态；订单 HTTP 对应 267 次 409。没有出现新的待核对告警。原数据已运行多轮流量，部分商品库存不足；不补库存、不删数据来美化错误数。

另有 53 条“订单已创建，但创建消息未确认发送”告警，集中在容器时间 17:04:13、17:04:14、17:04:25。只读逐个核对这 53 个订单号，processed_events 中有 53 条对应记录，说明它们随后实际完成了消费事务。最终 shopctl check 为 `ordered_quantity=2085212 sold_quantity=2085212 gap=0`。没有足够证据确定当时未及时确认的具体原因，不把它归因于 DNS，也不声称可靠消息问题已解决。这轮只用于有流量的资源观察，不是零错误性能基准。

<a id="phase-02"></a>

## 2026-10-23：阶段 2 开工记录

完整读完 1839 行计划书后，获取远端，确认工作区干净。按用户授权将 phase-01-inventory 快进合入 main 并推送，再从 main 创建 phase-02-order；基线 `3970868dae0e298c3cb0b1269a6800a473e2e7a7`。

环境实查：Go 1.27.1 windows/amd64，CGO_ENABLED=1；inventory-service、MySQL、Redis、Kafka 都 healthy。新库版本查询结果已写入 decisions。开工时发现“失败删除幂等占位”需要区分订单是否已经提交，已发出问题，先推进不依赖这个决定的缓存和服务治理。

只记录实际执行结果；没有执行的检查明确标为待验证。时间按用户时区 America/Los_Angeles。

阶段 2 的第一轮真实依赖测试：新增独立订单库 `opsdoctor_order_test`、销量库 `opsdoctor_sales_test`，Redis 商品测试用 DB 14，幂等测试用 DB 15。`go test -race` 的 9 个 Integration 测试全部通过：Redis 20 次并发占位只有一个执行者；同一订单消息并发处理 20 次后，销量为 3、去重记录为 1；销量溢出失败后去重记录回滚，随后能正确重放；订单详情和分页、空值缓存及 TTL、旧幂等所有者保护也通过。

开发中检查记录：首次运行 Go 测试因沙箱不能写机器上的编译缓存而失败，获得工具批准后正常运行。随后注释检查两次指出长函数缺少内部讲解，分别补充了 HTTP 错误映射顺序与取消后补偿的测试说明，重新检查通过。没有把这些失败记录成业务测试通过。

第一次完整 Compose 构建启动成功，七个容器均 healthy，loadgen 以 QPS=0 暂停。首次真实商品请求却降级，首次下单在约 804ms 后得到 `REQUEST_OUTCOME_UNKNOWN`。同键重复请求仍是待核对，没有重做。日志原因是 `deduct_outcome_unknown`，指标为 1。检查测试用户 880000002 没有订单，商品 499999 库存仍为 100。订单容器内 DNS、TCP 9001 和管理健康检查都正常，宿主机只读 RPC 也成功；稍后商品请求正常返回库存。

据此修正首次连接准备：`grpc.NewClient` 默认尚未建连，新增 `connectInventory` 在启动预算内等待 Ready，再开放 HTTP；增加内存握手与不可达超时测试。该次待核对 key 保留，没有删除或换键重做。修复后的冷启动与正式压测结果在后续记录。

修复后重建订单服务，第一次商品查询返回 stock.status=ok。新的独立测试用户 880000003 对商品 499998 购买两件，同键两次响应均为 201，订单编号同为 2000001，金额同为 3998 分。详情和列表返回该订单，数据库确认用户只有一笔订单、共两件。`shopctl check` 输出 `ordered_quantity=2000002 sold_quantity=2000002 gap=0`。

截至接线完成，`go test -race ./... -count=1 -timeout=180s` 全仓库通过；后续新增首次连接测试也已通过。`go vet ./...` 与 staticcheck 已通过。静态检查曾指出两处英文开头错误文本的 ST1005，改为中文开头后通过；这是实际遇到的格式问题，不是业务故障。

第一轮五分钟压测（目标 200 QPS，64 工作者）实际结果：300.0940009 秒，59119 次 HTTP 尝试，0 错误，880 次未发出，实际 197.00160557258243 QPS，P50=8ms、P99=453ms（一毫秒桶上界）。商品/下单/列表请求分别为 41405/11813/5901。12:44:56 UTC 的十秒窗口实际 116.466 QPS、P50=359ms、P99=2201ms，未发出 831 次；下个窗口又少发 49 次。不能把目标速率写成实际达到的速率。后续使用已有 `--workers 256` 配置重跑，保留两轮原始结果；第二轮已有部分缓存，不能把延迟差异全归因于工作者数量。

第二轮（宿主机，256 工作者）：300.0869976 秒，59782 次 HTTP 尝试，168 错误，302 次未发出，199.21556241395777 QPS，错误率 0.281021%，P50=14ms、P99=1229ms。商品/下单/列表为 41926/11992/5864。错误与少发集中在 12:50:10 UTC 的十秒窗口。同期订单日志出现缓存写入失败和商品查询耗尽三秒预算；MySQL 的 cgroup memory.events 读数为 max=5136、oom=0、oom_kill=0。它说明触及过限额，不能单凭这个累计值断言停顿根因。随后在 Compose 内部网络重测，排除 Windows 端口转发对生成路径的影响。

### 五分钟内部网络压测与消费者恢复（06:06–06:13）

命令：`docker compose --env-file .env -f deploy/docker-compose.yml exec -T loadgen /app/server --qps 200 --duration 5m --workers 256 --control :6071`。Compose 主 loadgen 保持 QPS=0，所以没有叠加第二股流量。全量数据保留，没有重置或补库存。

结果：300.011038541 秒、59998 次 HTTP 尝试、0 错误、0 dropped、实际 199.98597482205832 QPS、P50=5ms、P99=51ms，毫秒桶无溢出。三类请求 42194 / 11935 / 5869。有限时长最后不足一个调度周期及排空时间会使实际均值略低于目标 200，原始完成数没有补齐。

这轮最后约半分钟与消费者强制终止实验重叠，因此它不是全程无故障基准。原本准备另开十秒流量时发现内部 :6071 仍被五分钟进程占用，短进程因端口冲突退出，没有发出额外流量。之后直接使用现有流量完成积压验证。该操作顺序失误如实保留。

`kill -s SIGKILL order-worker` 后，三个分区 LAG 分别为 274、282、282。稍后 SQL 得到订单件数 2035307、销量 2034435；两次读取时流量仍在继续，因此 LAG 总数与 SQL 差值不要求相同。启动消费者后，分区位点均追平到 11930、11882、11837，LAG 全为 0。暂停并结束流量后，真实 `shopctl check` 输出 `ordered_quantity=2035650 sold_quantity=2035650 gap=0`。总量检查与真实 MySQL 重复事件测试共同证明本次没有多计数，不把一个总量相等推广成所有订单永远正确。

### 库存停服、恢复与优雅退出（06:14–06:16）

停止 inventory-service 后连续 15 次商品请求均返回 200、stock.status=unavailable。15 个独立测试下单中，前 10 次得到 409 REQUEST_OUTCOME_UNKNOWN，后 5 次在 Deduct 熔断后得到 503 UPSTREAM_UNAVAILABLE；后五次耗时分别为 5.432、5.276、6.723、5.636、5.337ms。同一个待核对键重发仍是 409，2.742ms，没有重新扣减。

日志含 order_id=0、sku_id=499996、idempotency_key 和 deduct_outcome_unknown 原因。`idempotency_requests_total{result="needs_review",service="order-service"}` 为 10。GetStock 与 Deduct 各自记录 closed→open；恢复库存后，各自记录 open→half-open→closed。商品重新返回 available=100。新的独立恢复验证订单返回 201，编号 2035650、金额 1999 分。

三个业务进程收到正常停止信号后都记录退出过程：订单“开始排空订单请求”与“订单入口已关闭，准备关闭下游连接”，drain_failed=false；消费者“等待当前订单消息完成”与“消费循环与管理入口已结束”；库存“开始排空库存请求”与“库存入口已关闭”。随后恢复靶场并暂停压测，不删除任何业务卷。

### 最终源码与说明检查

人工审查发现商品 handler 把数据库查询超时归成 500，已改为契约的 504 UPSTREAM_TIMEOUT，新增七个表格场景检查。库存展示失败仍是降级 200。这是验收后对错误分类的修正，没有改压测调度或成功请求路径。把客户端与调度的固定预算改成具名常量，并补充取值理由。

用去掉注释与 token.Pos 的 Go AST 加编译指令比较：main 中 71 个 Go 文件保持相同逻辑。仅排除本阶段新增 check 分派的 shopctl/app.go；新的阶段 2 文件不在这个旧代码比较里。三十五个正式读取的环境变量均能在 .env.example 和计划书找到。

最终检查顺序为 gofmt、go mod tidy、go vet、staticcheck、commentcheck、archcheck，再运行全仓库 race。全部退出 0，commentcheck=132 个非生成 Go 文件，archcheck 通过。无依赖轮为 67 个顶层测试通过、16 个真实集成测试跳过；注入六个专用 MySQL DSN 与 Redis 测试地址后为 83 个顶层测试通过、0 跳过、23 个有测试的包通过。新增 40 个顶层测试，地图含 134 个 Go 文件（含 2 个生成文件）。

学习材料九段 Go 最小示例补上包、导入及缺省上下文后编译通过。七段 PowerShell 代码块语法通过，CI YAML 和两个内嵌 Python 脚本语法通过。没有执行文档中的破坏实验来代替语法检查；实际故障测试另有前面的记录。

Redis 停服与恢复补验：原键重放订单 2035650，停止时响应 503，保留卷重启后仍为同一订单、同一金额 1999 分。最后 `shopctl check` 输出 `ordered_quantity=2035651 sold_quantity=2035651 gap=0`。压测保持暂停。

最终非 Markdown 文件 156 个，按路径排序、CRLF 归一化为 LF 后逐文件 SHA-256，再对“路径 + TAB + 文件摘要 + LF”的拼接结果计算 SHA-256：`c689a00cc74ca1a88494255b3c6fc2bd673715fbfb7c52a9c3118dec0ce3a89b`。文档追加 CI 结果不改变这个源码指纹。

提交前再检查：七个容器全部 healthy，loadgen 生效配置 qps=0、hot_sku_id=0。真实凭据扫描通过，.env 被 Git 忽略；相对文档链接有效，全部学习验收仍未勾选。链接脚本初次误把代码里的泛型签名与 Markdown 语法示例当作路径，排除代码块/行内代码后通过；没有因此修改合法示例。

### Linux CI 39961041773

[运行链接](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/39961041773)，源码提交 `4a467008d4da5968342523cf43530b879433ae94`。2026-10-23 06:25:14 创建，06:31:52 完成（America/Los_Angeles），结论 success。固定 ubuntu-24.04 / Go 1.27.1 的格式、tidy、vet、staticcheck、commentcheck、archcheck、无依赖 race、全新七容器 Compose、真实 MySQL/Redis race 和订单消息冒烟全部通过。

真实日志摘要：commentcheck=132；库存一百次并发扣减恰好五十成功、五十不足，最终零库存；small 造数重跑不覆盖修改后的库存。四个 HTTP 接口通过，同键两次返回订单 401，用户列表只有一笔订单。

CI 只做十秒、目标 20 QPS 的小规模冒烟：10.000962969 秒、199 次请求、0 错误、0 dropped、实际 19.898083876206783 QPS、P50=3ms、P99=25ms。三类请求为 140/36/23。最终 `ordered_quantity=438 sold_quantity=438 gap=0`。这些数字不替代本机全量五分钟结果。

随后仅补充 Markdown 的 CI 链接、实测摘要和对应文件行数，非 Markdown 源码指纹保持上面的值。阶段分支不合并 main，学习验收未勾选，未开始阶段 3。

阶段 1 记录见本文末尾；阶段 0 的结果保留当时版本，不用新版本的计数覆盖历史。

## 2026-10-23：开工检查

- 代码基线：main `3ebd194`，工作区干净，远端 `https://github.com/Li-Nepenthe/opsdoctor.git`。
- `go version`：`go version go1.27.1 windows/amd64`。
- `go env CGO_ENABLED CC`：`0`、`gcc`；未找到 gcc/clang 可执行文件。
- `docker version`、`docker compose version`：命令不存在；常见 Docker Desktop 安装位置也不存在。
- 原仓库 `git status --short --branch`：`## main`，无文件修改。
- 用户同意继续开发，本机 race 暂由固定版本 Linux CI 验证；Docker 由用户准备。

## 2026-10-23：环境补齐后的实测

记录汇总时间：01:50，America/Los_Angeles。检查时仍是 `phase-00-foundation` 的未提交工作树；下方源码指纹对应这一轮最终本地验证。对应源码提交为 de4459208791899131d5f67cbc8d42310b3948d7，远端记录如下。

- 刷新进程 PATH 后，`docker version --format '{{.Client.Version}} / {{.Server.Version}}'` 返回 `29.9.2 / 29.9.2`。
- `docker compose version` 返回 `v5.6.1`。
- `gcc --version` 返回 WinLibs MinGW-W64 GCC `16.2.0`；`go env CGO_ENABLED` 返回 `1`。
- `go list -m -json ...@latest`：Gin v1.12.0、MySQL driver v1.10.1、JWT v5.3.1、x/crypto v0.58.0、staticcheck v0.8.2。
- `gh api repos/actions/checkout/releases/latest --jq .tag_name`：v7.0.1；setup-go 同命令：v7.0.0。

### 失败记录与修复

1. `go test ./...` 初次运行：archcheck 的 embed 路径样例失败，因为未去掉 `//go:embed` 前缀。修复为逐个解析 embed pattern，随后用例通过。
2. commentcheck 初次自查发现 11 个长函数缺内部说明；补注释并拆分扫描职责。一次拆分后新函数又缺少内部说明，检查再次拒绝；补充后全部通过。
3. `docker compose -f deploy/docker-compose.yml up -d --wait ...` 未找到根目录 `.env`，报缺少 MySQL 密码变量；未创建容器。改为显式 `--env-file .env`。
4. 首次 Kafka 启动失败：`AccessDeniedException: /tmp/kraft-combined-logs/__cluster_metadata-0`。只读检查官方镜像发现 `/var/lib/kafka/data` 已归 appuser 所有；改用该目录，并修正本次新建空数据卷的属主。没有删除任何数据卷。

### 最终本地检查

按以下顺序执行；所有命令退出码均为 0，负例实验除外：

```text
gofmt -l internal tools
go vet ./...
staticcheck ./...
go run ./tools/commentcheck ./...
go run ./tools/archcheck ./...
go test -race ./... -count=1 -timeout=180s
go run ./tools/commentcheck -list ./...
```

- gofmt、vet、staticcheck：无诊断。
- commentcheck：`通过（32 个非生成 Go 文件）`。
- archcheck：`通过`。
- race：9 个包全部通过。源码共有 23 个顶层 Test 函数，另含表格子用例；没有把子用例数当成顶层测试数。
- 这一轮显式从 `.env` 注入 `TEST_MYSQL_DSN` 和 `TEST_MIGRATE_MYSQL_DSN`，两个集成测试均实际执行：UTC 会话、并发迁移只记一次、历史改写拒绝、已取消 context 后的锁释放与重获。
- 未注入测试 DSN 的独立运行也通过；集成测试会明确输出开启方法并跳过。HTTP 用 httptest 直接调用 Handler，不监听端口。
- 无大模型调用、无压测，无 QPS、P99、准确率或费用数据。

### 删除函数注释的独立验收

临时删除 `internal/platform/config/config.go` 中 `New` 的文档注释，执行 `go run ./tools/commentcheck ./...`：

```text
internal\platform\config\config.go:18:1: 函数或方法 New 缺少文档注释
commentcheck：发现 1 项问题
exit status 1
```

使用 `finally` 恢复原始字节，再运行检查：`commentcheck：通过（32 个非生成 Go 文件）`。这次实验没有留下工作区破坏。

### Compose 健康验收

```text
docker compose --env-file .env -f deploy/docker-compose.yml up -d --wait --wait-timeout 240
docker compose --env-file .env -f deploy/docker-compose.yml ps
```

| 服务 | 实际拉取并运行的镜像 | 状态 | 映射端口 |
|---|---|---|---|
| shop-mysql | mysql:8.4.13 | healthy | 127.0.0.1:3306 |
| redis | redis:8.10.2 | healthy | 127.0.0.1:6379 |
| kafka | apache/kafka:4.4.0 | healthy | 127.0.0.1:9092 |

进一步只把 `.env` 中的 `MYSQL_ROOT_PASSWORD`、`MYSQL_PASSWORD` 注入当前 PowerShell 进程，再执行计划书的原样命令 `docker compose -f deploy/docker-compose.yml up -d`：三个容器均返回 Running。没有把密码打印到终端或记录文件。

### CI

[GitHub Actions 运行 39929219914](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/39929219914) 的 quality 任务结论为 success，源码提交 de4459208791899131d5f67cbc8d42310b3948d7。ubuntu-24.04 上格式、模块一致性、vet、staticcheck、两个检查器、无外部依赖的 race、全新 Compose 健康检查、MySQL 集成 race 和清理步骤均成功。首次本机 Kafka 卷权限修正没有成为 CI 的人工前置条件，全新卷也实际启动成功。

### 最终本地验证的源码指纹

非 Markdown、非 .env 的已交付源码/配置文件，按相对路径排序，逐文件计算 SHA-256，再对路径与摘要的 LF 连接文本计算 SHA-256。值为：
a0e9a2b4db7821d8ac24099bbe210148b3c16e79fa33a068539b45da71cfde85

这包含 Go、SQL、Compose、CI、模块与示例配置，不包含后续补记结果的文档。后续提交只要这些文件不变，就对应同一份本地测试源码。

## 2026-10-23：注释可读性返工复验

验证对象：`phase-00-foundation` 在合并提交 `62126cff67130b70e70e4681466af54ea670f9dc` 之上的待提交返工。该合并引入 main 的 `6f92d0e1663acce2596e2194649188bc9a8f1b8a`，没有冲突。阶段 0 原有工程验收保留，学习验收仍未勾选。

变更范围：Go 中文注释、测试表格的注释与排版、学习材料、报告和本记录；`.env.example` 只删除 `CHAOS_ADDR` 与 `CHAOS_TOKEN`。没有修改 Go 的逻辑、测试输入或断言、SQL、依赖、Compose 和 CI 配置。

### 无行为改动的核对

临时核对程序使用 `git ls-files -z -- *.go` 枚举文件，分别读取 `git show d9b24f6:<路径>` 和工作树源码。用 `parser.ParseFile`（不加载注释，启用 `SkipObjectResolution`）解析，用 `ast.Fprint` 排除 `token.Pos` 字段后比较；另比较 `//go:`、`//line`、`// +build` 指令。32 个文件全部一致。临时程序在系统临时目录运行后删除，没有加入仓库。

最初尝试直接比较词法 token，因表格换成多行后增加合法的尾逗号而报告差异；随后改用上述 AST 对比，确认是排版差异，没有修改实现。这个区别保留在记录里，避免把工具的误判说成代码故障。

### 本机全部检查

```text
gofmt -w internal tools
gofmt -l internal tools
go vet ./...
C:\Users\Nepenthe\go\bin\staticcheck.exe ./...
go run ./tools/commentcheck ./...
go run ./tools/archcheck ./...
go test -race ./... -count=1 -timeout=180s -json
```

- gofmt 列表为空，vet/staticcheck 无诊断，退出码均为 0。staticcheck 不在当前 PATH，使用既有安装的绝对路径，没有更换版本。
- commentcheck：通过（32 个非生成 Go 文件）；archcheck：通过。
- race：9 个包全部通过。从被忽略的 `.env` 只读取两个 TEST DSN 并注入当前进程，没有输出密码；JSON 测试事件中 `TestOpenIntegration`、`TestApplyIntegration` 均为 pass，无 skip。
- `docker compose --env-file .env -f deploy/docker-compose.yml ps --format json`：shop-mysql、redis、kafka 的 Health 均为 healthy，State 均为 running。本轮未重建或清空数据卷。
- 学习材料第 5 节：把六段 Go 示例抽取到系统临时目录；前五个用 `go run` 执行，输出分别为 `11`、三个 `true`、`true true` 与 `true`、退出通知中文文本、`false` 与 `true 0`。第六段用 `go test <临时文件>` 编译通过，输出 `[no test files]`；未连接数据库执行丢弃连接实验。
- 示例验证脚本最初使用 Python 默认 cp1252 输出，打印第 4 个例子的中文时失败；改用 `python -X utf8` 后完成剩余验证。Go 示例本身没有运行错误。

### 本轮源码与配置指纹

取 `git ls-files` 中所有非 Markdown 文件（不包含被忽略的 `.env`），按相对路径排序；逐文件将 CRLF 统一为 LF 后计算 SHA-256，再把每行 `相对路径 + 制表符 + 文件摘要 + LF` 连接起来求 SHA-256。用于标识上述已验证工作树，值为：

`d10bb5b5c85a89729749d78a5850a68317fb29dc5511216035f23ab656af6bc8`

前文的首次验收指纹和 CI 记录属于原始交付，作为历史保留；本轮以这里的验证对象和指纹为准。分支推送后的运行可在 [phase-00-foundation 的 Actions 记录](https://github.com/Li-Nepenthe/opsdoctor/actions?query=branch%3Aphase-00-foundation) 查看。

<a id="phase-01"></a>

## 2026-10-23：阶段 1 的开发与验收

代码版本：`phase-01-inventory` 工作树，基线 main `3aa6c99`（包含阶段 0 的 `4404d78`）。最终源码指纹与远端 CI 记录补在本节末尾。下列耗时来自本机 Windows 10 / Go 1.27.1 / MySQL 8.4.13；不是通用性能承诺。

### 实际开发顺序与发现的问题

1. 用户批准阶段 0 合入 main 后，执行 fast-forward 合并和推送，再创建阶段 1 分支。先读完整计划书和当前进度，没有勾选学习验收。
2. 先写 proto、迁移、领域类型与条件更新仓储，再接 gRPC、健康检查、进程退出流程，随后实现 seed 和 CLI，补单元与真实 MySQL 测试。
3. buf lint/generate 通过；生成物保存在 gen。安装的是固定版本 Go 工具，没有安装系统软件。构建镜像 manifest 与实际拉取均成功。
4. 原 commentcheck 要求每个包有 doc.go，与 cmd 薄入口只有 main.go 的规范冲突；先补 cmd/main.go 包说明例外及测试。后续 main 的新规范也明确了这个例外。
5. 检查退出流程时把 Serve 错误逐个过滤后再 errors.Join，避免一组错误里包含正常关闭哨兵时把另一个真实故障一起忽略。这是代码审查发现的问题，没有编造线上故障。
6. 写 Docker 配置并启动四个容器；运行 full seed、重复执行、RPC 手工扣减，再停止库存服务完成删索引/恢复索引实验。
7. 用户发来三层规范后，暂存本阶段未提交文件，合入 main `3aa6c99`，恢复工作树，无冲突；先升级 commentcheck，再改已有说明和目录导航。
8. 新增 cmd/README.md 后，archcheck 报“cmd/<程序>/ 只能包含 main.go”。因阶段 0 的逻辑范围受到限制，暂停此项并询问；用户批准精确放行根 cmd/README.md，要求同步计划书和决定记录。修复后正反例及全量检查通过。

### 数据初始化原始结果

先按 README 从 .env 注入 MYSQL_DSN；日志不输出 DSN。

```text
go run ./cmd/shopctl seed
products=500000 stock=500000 orders=2000000 inserted_products=500000 skipped=false seed_elapsed=1m15.631s total_elapsed=1m15.651s

go run ./cmd/shopctl seed
products=500000 stock=500000 orders=2000000 inserted_products=0 skipped=true seed_elapsed=499ms total_elapsed=538ms
```

SQL COUNT 实查：products=500000，stock=500000，orders=2000000，sales 行数=500000，SUM(sold)=2000000，processed_events=2000000。耗时区分 seed 内部计时和 CLI 连接、迁移在内的总计时。

RPC 实查：`stock get --sku 1` 得到 100；`stock deduct --sku 1 --quantity 3` 成功；再次 get 得到 97。实验后在停服期间只把这次修改的 SKU 1 恢复为 100；没有重置其他库存或删除数据卷。

### EXPLAIN 原始观察

库存服务先停止，避免索引实验影响 RPC。MySQL CLI 会话隔离级别为 REPEATABLE-READ、时区 SYSTEM；这是 CLI 的会话，应用连接仍由 mysqlx 设置为 +00:00，集成测试另外验证。

```sql
EXPLAIN SELECT sku_id, available FROM stock WHERE sku_id = 250000 AND available >= 1;
ALTER TABLE stock DROP INDEX uk_stock_sku;
EXPLAIN SELECT sku_id, available FROM stock WHERE sku_id = 250000 AND available >= 1;
ALTER TABLE stock ADD UNIQUE INDEX uk_stock_sku(sku_id);
EXPLAIN SELECT sku_id, available FROM stock WHERE sku_id = 250000 AND available >= 1;
```

| 状态 | id | select_type | table | partitions | type | possible_keys | key | key_len | ref | rows | filtered | Extra |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 删除前 | 1 | SIMPLE | stock | NULL | const | uk_stock_sku | uk_stock_sku | 8 | const | 1 | 100.00 | NULL |
| 删除后 | 1 | SIMPLE | stock | NULL | ALL | NULL | NULL | NULL | NULL | 498780 | 3.33 | Using where |
| 恢复后 | 1 | SIMPLE | stock | NULL | const | uk_stock_sku | uk_stock_sku | 8 | const | 1 | 100.00 | NULL |

`rows` 是优化器估算，不是实际 COUNT。finally 路径恢复索引，SHOW INDEX 确认 PRIMARY(id) 和唯一的 uk_stock_sku(sku_id) 均存在，再启动库存容器并确认 healthy。

停服日志实际出现“收到退出信号，开始排空库存请求”和“库存入口已关闭，准备关闭数据库连接池”。普通请求日志包含 request_id、RPC 方法、状态码与耗时。

### 测试与静态检查

从 .env 白名单注入四个 TEST DSN，各自指向专用测试库。执行：

```text
gofmt -w internal tools
gofmt -l cmd gen internal tools
go vet ./...
staticcheck ./...
go run ./tools/commentcheck ./...
go run ./tools/archcheck ./...
go test -race ./... -count=1 -timeout=180s -json
go run ./tools/commentcheck -list ./...
go run ./tools/commentcheck -map ./...
```

最终本地结果：全部退出 0；格式、vet、staticcheck 无诊断；commentcheck 通过（70 个非生成 Go 文件）；archcheck 通过。JSON 输出统计 43 个顶层测试、15 个通过的包，没有 skipped 测试。72 个 Go 文件进入地图，其中 2 个是生成物。

```text
100 次并发扣减：成功=50，不足=50，最终库存=0
100ms deadline：客户端=DeadlineExceeded，服务端=context deadline exceeded，总等待=100.5768ms（原查询 5s）
small：products=100 orders=400；重跑 skipped=true，已修改库存保持 7
```

seed 回滚测试制造订单主键冲突，确认本批先写的商品也回滚；已取消 context 不写入。deadline 测试验证 Go handler 及时返回、驱动停止等待，不据此宣称 MySQL 远端查询瞬间终止或写入必定回滚。

三层规范升级前的同一业务实现也跑过完整 race：40 个顶层测试、15 个包，100ms deadline 实测 100.9908ms，服务端感知 context canceled。新增 3 个文件说明/清单/地图测试后才是上面的 43 个；两轮计数不混用。

注释范围核验：用 go/parser 去掉注释和 token.Pos 后比较 main 与工作树 AST，并单独比较 //go:、//line、旧式 build 指令。26 个阶段 0 文件完全一致。仅排除用户要求扩展的 commentcheck 原有 4 个文件、以及用户额外批准的 archcheck/check.go 和 check_test.go；其余阶段 0 逻辑未变。

### 最终收尾核验（04:05–04:06，America/Los_Angeles）

人工复读最后几条注释后，再按相同顺序跑完整检查：gofmt/vet/staticcheck/commentcheck/archcheck/AST 比较及全量 race 都通过。43 个顶层测试、15 个有测试的包通过，无测试函数跳过；另 5 个包为 no test files（正常构建，不是集成测试被跳过）。本轮 deadline 输出为 100.5017ms、客户端 DeadlineExceeded、服务端 context canceled，两种服务端取消原因都符合协议取消传播。

重新构建当前业务源码的 Compose 成功，`ps --format '{{.Service}} {{.Health}}'` 得到 inventory-service、kafka、redis、shop-mysql 全部 healthy；`Invoke-RestMethod http://127.0.0.1:6062/health` 得到 ok。文档 PowerShell 代码块只做语法解析，没有执行其中的破坏实验；7 段均无语法错误。文件地图和相对文档链接检查通过，忽略的 .env 未纳入 Git，未发现本机真实密码或私钥/令牌模式进入待提交文件。

最终非 Markdown 源文件 93 个，归一化 CRLF 为 LF 后，按路径排序计算每文件 SHA-256，再对 `路径 + TAB + 文件哈希 + LF` 拼接结果计算 SHA-256：`dcda6bee0d7937567b82434b74028c844cca48fc3ff7068c8f7297b3938bff7d`。这个指纹包含代码、测试、协议、部署和 CI，文档后补 CI 链接不会改变它。

### Linux CI 39943928805

[运行链接](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/39943928805)，源码提交 `54bd24f024001b57a5b4faa9d974c619957c24d8`。2026-10-23 04:07:53 创建，04:11:29 完成（America/Los_Angeles），结论 success。ubuntu-24.04 / Go 1.27.1 的格式、tidy、vet、staticcheck、两种规则、无外部依赖 race、全新 Compose、真实 MySQL race 与 CLI 冒烟全部通过。

日志摘录：

```text
100 次并发扣减：成功=50，不足=50，最终库存=0
100ms deadline：客户端=DeadlineExceeded，服务端=context deadline exceeded，总等待=102.604738ms（原查询 5s）
small：products=100 orders=400；重跑 skipped=true，已修改库存保持 7
products=100 stock=100 orders=400 inserted_products=100 skipped=false seed_elapsed=16ms total_elapsed=20ms
products=100 stock=100 orders=400 inserted_products=0 skipped=true seed_elapsed=3ms total_elapsed=6ms
sku_id=1 available=100
sku_id=1 deducted=1
sku_id=1 available=99
```

CI 只跑 small，不把它的 20ms 当作全量造数时间。完整规模的 75.651 秒来自前面的本机记录。收尾另对学习材料 10 段 Go 示例在临时模块中补上必要包/导入进行编译：全部通过；t.Context/Cleanup 示例测试也通过。随后只修改文档记录与文件行数清单，不改业务源码或检查工具。
