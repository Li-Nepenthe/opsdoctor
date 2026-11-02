# OpsDoctor 架构说明

OpsDoctor 把三件事放在同一个仓库：运行中的电商靶场、受权限约束的诊断医生、知道标准答案的故障注入与评测器。靶场产生真实症状，医生只能从观测数据取证，评测器独立比较结果。先看 [展示入口](../README.md)，运行命令见 [操作手册](operations.md)，找代码时从 [代码地图](代码地图.md) 进入。

本文描述已经实现的系统。用户在 2026-10-30 决定为尽早投递先做阶段 10，阶段 9 推迟而非取消；2026-11-02 确认继续执行。因此当前真实接入对象只有 shop，日志后端只有 Loki，尚未用第二个真实系统验证泛化。

## 总体结构

```mermaid
flowchart TD
    Load[loadgen 模拟用户] --> Order[order-service / HTTP]
    Order --> Redis[Redis 商品缓存与幂等]
    Order -->|gRPC| Stock[inventory-service]
    Stock --> ShopDB[(shop MySQL)]
    Order --> ShopDB
    Order --> Kafka[Kafka order.created]
    Kafka --> Worker[order-worker]
    Worker --> ShopDB
    Order & Stock & Worker --> Obs[Prometheus / Jaeger / Loki]
    Obs --> Grafana[Grafana]
    Obs --> Alerts[Alertmanager]
    Alerts --> Doctor[doctor 接收并去重]
    Doctor --> Agent[Agent 循环]
    Agent <--> Model[Mock / 显式授权的真实模型]
    Agent --> Tools[绑定当前系统的工具]
    Tools --> Obs
    Tools --> ReadOnly[MySQL / Redis / pprof 只读检查]
    Agent --> Approval[保存审批单]
    Human[已登录的人] -->|批准或拒绝| Approval
    Approval --> Limited[受限建索引 / 调池 / 预热]
    Doctor & Agent & Approval --> DoctorDB[(独立 doctor MySQL)]
    DoctorDB --> SSE[SSE / 页面 / 历史回看]
    Eval[宿主机 chaos 与 eval] -->|注入与恢复| Order
    Eval -->|仅评测器可读| Answers[eval/scenarios 标准答案]
    Eval -->|安排诊断并读取结果| Doctor
```

图里的箭头表示数据流或调用，不表示跨越所有权限。Toxiproxy 位于订单到库存、库存到 MySQL、订单到 Redis 的链路中；它由宿主机注入器控制，医生没有这项权限。实际监听与映射以 [Compose](../deploy/docker-compose.yml) 为准，宿主机端口只绑定回环地址。

## 代码怎样分工

| 位置 | 解决的问题 | 阅读入口 |
|---|---|---|
| `cmd/` | 每个程序的薄入口，把退出错误交给操作系统 | [程序说明](../cmd/README.md) |
| `internal/platform/` | 配置、日志、HTTP/gRPC、迁移、连接池、观测与停机等通用能力 | [基础设施说明](../internal/platform/README.md) |
| `internal/shop/` | 商品、库存、订单、销量，以及服务组装 | [靶场说明](../internal/shop/README.md) |
| `internal/doctor/` | 数据源、工具、模型、循环、调度、审批、页面、MCP 与持久化 | [医生说明](../internal/doctor/README.md) |
| `internal/chaos/` | 用真实结构、配置、网络和流量变化注入故障，并恢复基线 | [注入器说明](../internal/chaos/README.md) |
| `internal/eval/` | 冻结输入、配对运行、评分、费用与版本审计、结果导出 | [评测器说明](../internal/eval/README.md) |
| `tools/` | 开发时检查注释、依赖与崩溃恢复 | [工具说明](../tools/README.md) |

业务包按接口、业务和数据访问分层。以库存为例，[gRPC 适配](../internal/shop/inventory/grpc_server.go) 把 protobuf 请求变为领域参数，[业务层](../internal/shop/inventory/service.go) 检查规则，[仓储](../internal/shop/inventory/repository.go) 执行 SQL。业务层定义自己需要的接口，测试可以用假实现替换仓储。Go 按方法集合满足接口，不需要 Java 的 `implements` 声明。

医生不得导入 shop、chaos、eval 的业务实现。双方通过明确的 HTTP、gRPC 和数据库检查协议交互，[archcheck](../tools/archcheck/main.go) 会检查这条边界。医生容器不挂载标准答案，也不持有 Docker socket。Alloy 采集 Docker 日志所需的挂载属于采集器，不能因此说医生有 Docker 权限。

## 靶场的请求、缓存和消息

商品详情先查 Redis，未命中再读 MySQL 并填回缓存，这叫 Cache Aside。空值缓存减少不存在商品的重复查询；TTL 加随机抖动把过期时间打散；singleflight 合并同一进程中同一商品的并发回源。库存单独走 gRPC，库存不可用时商品信息仍返回，并明确标注库存不可用。实现见 [商品服务](../internal/shop/product/service.go) 和 [缓存](../internal/shop/product/cache.go)。

下单先获取 Redis 幂等占位，再查价格、扣库存、写订单并发布消息。库存扣减用带余量条件的 UPDATE，在数据库行锁内完成判断与修改，防止并发超卖。写订单失败时尝试归还库存；结果不确定时保留幂等记录，返回 `REQUEST_OUTCOME_UNKNOWN`，供人核对。实现见 [下单流程](../internal/shop/order/service.go)、[幂等状态](../internal/shop/order/idempotency.go) 和 [库存 SQL](../internal/shop/inventory/repository.go)。

order-worker 消费消息后，在同一事务中插入 `processed_events` 并累加 `product_sales`，事务成功后才提交 Kafka 位点。崩溃可能使消息再次出现，已处理事件表负责消除重复副作用。数据库提交与 Kafka 发布之间仍有一致性窗口：没有实现 Outbox，不能保证订单与消息绝对一致。`shopctl check` 核对总量，也不能单独证明逐商品无误。见 [销量事务](../internal/shop/sales/repository.go) 和 [消费者](../internal/shop/app/orderworker/app.go)。

## 观测数据怎样到医生

指标由 Prometheus 定时采集；Jaeger 接收 OpenTelemetry 的调用链；slog 输出的 JSON 日志由 Alloy 收集到 Loki。HTTP、gRPC、SQL、Redis 和消息处理围绕请求建立 span；Kafka 的上下文通过消息头手动注入与提取。日志携带 trace ID，方便将不同服务的同一请求串起来。见 [观测基础设施](../internal/platform/telemetry/doc.go)、[Kafka 传播](../internal/platform/telemetry/kafka.go)。

告警描述症状，例如延迟、错误或积压，不在告警名里提前给出“缺索引”等答案。实际规则见 [Prometheus 规则说明](../deploy/prometheus/rules/README.md)。监控接口健康只说明接线的一部分；还要检查 exporter 是否能登录中间件，以及真实 trace 和日志能否查到。阶段 3 曾用这类端到端检查发现 gRPC DNS TXT 查询等待，见 [阶段 3 汇报](reports/phase-03-observability.md) 和台账 P3-18 至 P3-26。

医生的数据源接口把“查什么”和“后端怎样表达”分开。[source](../internal/doctor/source/source.go) 定义统一类型，工具只依赖这些接口，具体适配器处理 Prometheus、Jaeger、Loki、MySQL、Redis 与 pprof 的格式。

**当前日志适配器只有 [Loki](../internal/doctor/source/loki/client.go)。** 工具传服务、级别、关键词和时间条件，适配器转义字符串后生成 LogQL，不接受模型给出的任意 LogQL。OpenSearch 适配器和两个后端的共用契约测试属于推迟的阶段 9，目前没有实现。也不支持 PostgreSQL、SkyWalking 和非 Go 服务的运行时分析。以后补做阶段 9 时，需要同步更新本文日志适配器说明、README 支持后端及限制、第二张结果表、简历草稿和数据台账。

## 核心流程一：告警 → 诊断 → 报告

```mermaid
sequenceDiagram
    participant AM as Alertmanager
    participant API as doctor HTTP
    participant DB as doctor MySQL
    participant Run as 调度器
    participant A as Agent
    participant M as Mock/模型
    participant T as 当前系统工具
    participant UI as 页面/SSE
    AM->>API: 带系统独立令牌的 firing
    API->>DB: 事务保存通知、故障组与 pending 任务
    Run->>DB: 有并发名额时领取租约
    Run->>A: 绑定系统、配置、预算与已存步骤
    loop 直到报告或终止条件
        A->>M: 固定提示、说明书、告警、已有结果
        M-->>A: 工具名、JSON 参数、调用编号
        A->>T: 校验后执行只读工具
        T-->>A: 有界结果或可解释的错误
        A->>DB: 持久化步骤与调用审计
        DB-->>UI: 按步骤编号补发，实时通知促使刷新
    end
    M-->>A: submit_report
    A->>A: 校验结构、组件与真实证据步骤
    A->>DB: 保存报告与终态
    DB-->>UI: 展示结论、证据、建议与未知项
```

1. [HTTP 路由](../internal/doctor/httpapi/router.go) 验证目标系统的令牌，[告警接收器](../internal/doctor/alerts/service.go) 检查通知，再由 [store](../internal/doctor/store/alerts.go) 在事务里归并。报告生成后，故障组后续通知仍加入同一任务；整组恢复后再经过观察期，避免为同一事故反复诊断。观察期也可能合并真正的新事故，用户可以手动诊断。
2. [调度器](../internal/doctor/run/scheduler.go) 按总名额和每系统名额安排任务。数据库租约证明当前执行权，续约失败会在租约失效前停止本地执行。等待审批时释放全局名额，但仍占住该系统的名额。
3. [Agent](../internal/doctor/agent/loop.go) 调用模型，由 [calls](../internal/doctor/agent/calls.go) 并行查询。普通工具失败变为工具结果，便于模型纠正；审计保存失败则停止，防止出现做过却无记录的步骤。最终报告必须引用当前诊断里保存的真实步骤。
4. [写操作申请](../internal/doctor/approval/service.go) 先生成审批单，保存具体参数、语句预览、理由、证据、风险和撤销方法。登录用户批准后，执行器只读原单参数。恢复时核对索引、实例修订号或缓存状态，不能把超时当作没执行。
5. [SSE](../internal/doctor/httpapi/stream.go) 是服务器到页面的单向事件流。数据库是事实来源，`Last-Event-ID` 对应持久化步骤号；慢客户端断线后可以补发。Cookie 登录的修改接口另做同源和 CSRF 检查；动态内容以文本显示。

`context` 将取消通知沿调度器、Agent、模型 HTTP 请求、数据源查询往下传。进程重启后从已保存的请求与结果重建对话；已有只读结果原样回放，缺少时才补查。租约、预算、模型名和已花费用都不能随重启归零。实现分别见 [回放](../internal/doctor/agent/replay.go)、[执行权](../internal/doctor/store/leases.go)、[回复恢复](../internal/doctor/store/model_recovery.go)。

## 权限与资源边界

| 边界 | 代码怎样执行 | 证明入口 |
|---|---|---|
| 工具与系统 | 固定地址、有限参数和注册表；工具实例预先绑定系统；无任意 SQL、URL 或命令工具 | [工具校验测试](../internal/doctor/tools/catalog_test.go)、[目标隔离测试](../internal/doctor/target/catalog_test.go)、[SELECT 校验测试](../internal/doctor/source/select_test.go) |
| 人工审批 | 写操作先存单，条件更新只允许一次决定；实际执行只用原单 | [审批单元测试](../internal/doctor/approval/service_test.go)、[数据库争抢测试](../internal/doctor/store/approval_integration_test.go) |
| 独立账号 | 只读 SQL/Redis、独立 INDEX 账号、固定运维接口令牌 | [MySQL 权限](../internal/doctor/source/mysqlinspect/permissions_integration_test.go)、[Redis ACL](../internal/doctor/source/redisinspect/permissions_integration_test.go)、[DDL 权限](../internal/doctor/approval/permissions_integration_test.go) |
| 资源预算 | 查询超时、结果截断、连接数、并发数、轮数、token、费用和活动时长共同限制 | [注册表测试](../internal/doctor/tools/registry_test.go)、[循环预算测试](../internal/doctor/agent/budget_test.go)、[调度测试](../internal/doctor/run/scheduler_test.go) |

MySQL 只读账号因 EXPLAIN 需要而有业务表 SELECT 权限，工具不会返回业务数据行；这两件事必须区分。INDEX 权限本身也允许删除索引，账号无法按医生生成的前缀细分授权；代码只提供受限的创建，撤销由人完成。

模型完全受诱导时，仍只能查看当前系统允许的证据、消耗有上限的资源并申请有限修改。误批建索引会消耗数据库资源，误批缩小连接池可能让服务变慢；“可撤销”不等于没有业务影响。真实模型会接收脱敏后的说明书、告警和工具结果，日志与 SQL 仍可能包含业务值。当前没有角色隔离，任何登录用户都可审批，脱敏也不是完整个人信息脱敏；生产使用还需要补齐这些能力。

MCP 通过 `/mcp/{target}` 提供同一批只读工具，独立令牌绑定一个系统；既不开放写工具，也不开放审批。审计开始记录失败时不访问后端。见 [MCP 实现](../internal/doctor/mcpserver/doc.go) 和 [接入说明](mcp.md)。

## 持久化与费用

shop 和 doctor 使用不同 MySQL 实例。前者保存业务数据，后者保存用户、告警、任务、步骤、审批、MCP 审计、模型尝试、预算和评测结果。doctor 能写自己的库，不意味着能任意写 shop。表结构沿 [医生迁移](../internal/doctor/store/migrations) 和 [靶场迁移](../internal/shop/migrations) 阅读。

模型请求前在持久化事务中预留费用和批次 token，确认用量后结算，结果未知时保留预留；单次调查、全局账户和批次预算共同约束花费。供应商、模型、账户、币种和单价随尝试保存，重启不清除累计。费用算法见 [模型客户端](../internal/doctor/llm/doc.go) 与 [账本](../internal/doctor/store/budget.go)。

美元按项目用量账本计：每次请求实际 token 用量乘以 2026-10-29 核对过的官方单价，分档结算，未知请求保留预留。平台账单因 Key 与其他用途共用，无法单独核对。美元与人民币分开，不能相加。DeepSeek 余额差 ¥0.769756 尚未归因，详见 [数据台账](数据台账.md) P8-190、P8-197；不猜差额原因。本阶段的演示和检查只用 Mock。

## 核心流程二：注入 → 评测

```mermaid
flowchart LR
    Freeze[冻结版本、标准、输入与随机顺序] --> Healthy[核对健康与流量基线]
    Healthy --> Inject[chaos 注入真实原因]
    Inject --> Verify[确认症状、捕获同一故障输入]
    Verify --> Paired[按保存顺序运行五配置]
    Paired --> Report[保留报告、步骤、预算及失败]
    Report --> Grade[组件规则 / DeepSeek机制与证据评分]
    Grade --> Restore[按原场景恢复]
    Restore --> Wait[健康、冷却、故障开放期关闭]
    Wait --> Next[下一轮或导出]
```

冻结清单先保存代码哈希、提示、手册、评分与模型版本，以及各轮配置顺序。E0 没有查证工具，E1 使用完整工具，E2 增加通用手册检索，E3 去掉调用链工具；DeepSeek E1 在同一次故障上作为另一配置比较。顺序随机并留档，减少先后时间固定偏向某组的影响。医生不知道场景答案，评测器才读取它。见 [冻结清单](reports/phase-08-formal-manifest.json)、[配对执行](../internal/eval/runner/paired.go) 和 [自动流程](../internal/eval/runner/automatic.go)。

中断后续读原轮次和槽位，保存过的输入、终态与首次盲评编号不覆盖。正常可靠性失败与格式、推理和预算失败分开；指定校时取消的补跑是经用户批准的单独例外，不推广成随意重跑。评测时写审批会被拒绝，执行器也再次拦截；恢复由宿主机评测器完成。见 [结果存储](../internal/doctor/store/eval_results.go)、[可靠性分类](../internal/doctor/store/eval_reliability.go)、[版本修订审计](reports/phase-08-execution-versions.json)。

阶段 8 冻结代码始于 `5e5d5a1`，后续仅获准的基础设施修订为 `ca2ec68` 与 `89945fd`；提示 v8.0、手册 v1.1、评分 v1.1 保持冻结。正式自建靶场 S01–S13 每场景 3 次，195 份报告、181 份有效评分、14 份评分失败，另保留原取消；这些是台账 P8-38、P8-191、P8-194 的实验范围，不能称为生产准确率。S10 带工具的 12 份全部把连接池当根因，见 P8-149、P8-153、P8-157 和 [失败分析](reports/phase-08-failure-analysis.md)。

**DeepSeek 评委与 Claude 交叉评分的一致率**为全量机制 17/20（85%）、去掉 E0 为 12/15（80%），分别见台账 P8-199、P8-201。证据判定中，DeepSeek 评委比 Claude 多给 11 项、少给 4 项，见 P8-202。未做人工校准；两个模型可能有共同偏差。评分尺度不一致和评分失败都不能被一致率掩盖。[完整交叉评分](reports/phase-08-cross-scoring.md) 保留了逐份理由。

S12 的攻击文字没有被医生实际读到，见台账 P8-178、P8-204。当前防注入只有权限、审批与隔离测试作证据，不能把未暴露条件下的零写执行说成实际抗攻击验证通过。

## 当前取舍

采用单实例医生加 MySQL，减少分布式调度和消息基础设施的复杂度；代价是没有高可用。原生页面和 SSE 足够展示诊断过程，代价是没有复杂前端交互。通用手册用本地关键词检索，容易审计和冻结，代价是检索覆盖有限。工具接口让后端差异留在适配器，但“换后端只改适配器”的真实外部验证仍待阶段 9。

故障实验与评测既是项目能力，也是约束：它能解释这套靶场中为什么判断成功或失败，尚不能支持对陌生系统、长期生产运行或安全攻击的宽泛结论。
