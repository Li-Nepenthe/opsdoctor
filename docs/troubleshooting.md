# OpsDoctor 自身排障

本页处理项目怎么启动、接线和恢复。医生检索的通用手册在 `docs/runbooks/`，已冻结为 v1.1；这里的维护说明不会成为阶段 8 的模型输入。首次运行按 [README](../README.md)，完整命令按 [操作手册](operations.md)。

## 先确认自己正在操作哪一套环境

Compose 文件写了 `name: opsdoctor`，换一个目录不会自动获得新数据。下面的示例显式使用 `-p opsdoctor-fresh`，只针对隔离验收项目；排查原项目时把它替换成 `opsdoctor`。操作前确认项目标签、端口和卷，两套环境不能同时运行。

```powershell
docker ps -a --format 'table {{.Names}}\t{{.Status}}\t{{.Labels}}'
docker volume ls
docker compose -p opsdoctor-fresh --env-file .env -f deploy/docker-compose.yml ps --all
docker compose -p opsdoctor-fresh --env-file .env -f deploy/docker-compose.yml logs --no-color --tail 100 order-worker kafka shop-mysql
```

bash 使用同样的 Docker 命令。向别人提供日志前先检查是否含密钥、令牌或业务数据；不要打印 `.env`、完整 Compose 展开配置或连接串。

本项目验收保留所有卷，使用 `stop` 停回测试环境。不要用删除卷或清理整个 Docker 的方式修复问题，尤其不能删除原 `opsdoctor` 的 doctor-mysql 卷：其中保存阶段 8 原始数据。也不要操作 `qq-astrbot`、`qq-napcat`。测试项目的卷是否删除由用户之后决定。

## 启动失败先看哪个证据

| 现象 | 先看什么 | 根据证据处理 |
|---|---|---|
| `port is already allocated` | `docker ps -a` 的端口、当前项目和宿主进程 | 先停回占用同端口的那一套环境；保留其卷。不要临时换端口后忘记更新接入配置 |
| `order-worker exited (1)` | 同一次启动的 worker、Kafka、shop-mysql 日志及容器状态 | 区分迁移、主题初始化、消费者启动失败；保留首个错误，不以加重启策略代替定位 |
| MySQL 第一次初始化失败 | `shop-mysql` / `doctor-mysql` 的入口日志 | 看失败的初始化脚本和权限；已有卷不会重新执行初始化脚本。不要为了让脚本再跑而删业务卷 |
| MySQL `Access denied` | 发生错误的是业务、只读、DDL 还是 doctor 连接 | 核对对应环境变量名和授权脚本；独立账号不可互换，避免把业务密码交给医生 |
| 容器 running 但 unhealthy | 该服务 healthcheck、管理端口日志、依赖健康 | running 只代表进程存在；逐项确认真正依赖，不能只增加等待时间 |
| Docker API 连接失败 | Docker Desktop 状态、当前 Docker context、命令权限 | 先恢复 Docker 引擎访问。引擎不可达时，应用日志也不足以解释业务根因 |

阶段 8 的 [CI 41447608555](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/41447608555) 曾出现空环境 worker 退出，当时只保留 MySQL 日志，无法确认根因。历史失败保留在台账 P8-207。阶段 10 对照实验与最终修复以 [notes](notes.md) 中的原始日志和本阶段汇报为准；下面给出阅读顺序，避免把历史未确认原因当成已知事实。

启动链路先读 [Compose](../deploy/docker-compose.yml) 中 Kafka 的健康条件，再读 [order-worker 组装](../internal/shop/app/orderworker/app.go) 中数据库迁移、主题确保与消费者创建的顺序。Kafka 接受端口连接、能够返回元数据、主题已经可消费，是不同的就绪条件。定位时要保留 Kafka 与 worker 的同一时间窗口日志，不能只凭最终 unhealthy 结论推断。

## 首次空库没有商品，或只读工具报表不存在

首次启动先暂停流量，等服务迁移完成，再按 README 运行造数与精确授权。`shopctl seed --small` 适合免费页面演示和 CI，小规模数据不能替代故障性能验收；完整故障实验按操作手册的正式基线准备。造数发现已有数据会跳过，不会把旧数据重置。

只有数据库健康还不够，业务表由应用迁移创建。给具体表授权的脚本要等迁移后执行，见 [操作手册](operations.md) 的首次启动步骤与 [MySQL 初始化说明](../deploy/mysql/init/README.md)。授权不足时先核对授权，不改用 root 或扩大权限来“解决”。

## 医生启动了，但没有诊断

先登录页面看运行状态，确认 `LLM_PROVIDER=mock`，再手动创建症状说明。Mock 只演示调用、审计和报告流程，页面必须出现“Mock 模式，不是真实诊断”；不能据 Mock 报告评价根因能力。真实效果查看 [阶段 8 冻结结果](reports/phase-08-formal-results.md)，本阶段不调用付费接口。

如果是自动告警没有建任务，按顺序看 Prometheus 告警状态、Alertmanager 通知错误、目标系统名与 `SHOP_ALERT_TOKEN` 的配置、医生告警接收日志。令牌只在环境变量中核对，不复制到命令行或文档。

有通知但没有新任务，可能是同一故障组被归并、还在恢复观察期、评测模式只捕获通知、排队超过有效期，或全局费用守卫阻止真实任务。查看目标系统的 `fault.open`、`fault.recovery_until`、诊断状态与运行状态；不要连续创建真实任务来试。去重设计见 [故障开放期](../internal/doctor/store/faults.go)，调度原因见 [scheduler](../internal/doctor/run/scheduler.go)。

## 工具不可用、结果为空或被截断

先看目标系统的工具菜单。未配置的数据源会降级为不可用工具，和“后端配置了但查询失败”不同。当前日志只支持 Loki；不能填一个 OpenSearch 地址来代替，阶段 9 适配器尚未实现。

工具结果为空时先核对服务名、时间范围和实际后端是否有数据，再核对过滤条件。日志查询默认围绕故障时间，过去故障可能进入回看窗口；报告必须区分历史与当前。S12 已说明这种限制：攻击文字未进入医生实际看到的工具结果，不能把空结果当成防注入成功。

结果含截断标记时，缩小时间窗、服务或关键词再查。不要扩大工具硬上限来完成演示；它限制故障期资源消耗和模型上下文。实现见 [工具注册表](../internal/doctor/tools/registry.go)、[查询窗口](../internal/doctor/tools/window.go)。

## 有 trace ID，却在 Jaeger 查不到

按顺序确认请求是否被采样、SDK 是否导出成功、Jaeger 是否重启丢失内存数据、搜索时间范围是否正确。查不到不等于请求没发生。消息消费端还要核对 Kafka header 是否携带 traceparent，以及消费者是否提取。

Grafana 的 Jaeger 数据源健康失败时看具体 HTTP 路径。项目固定版本存在经过验证的兼容性例外，不能随意把镜像换成最新版；阶段 3 的失败与修复见台账 P3-9、P3-11 及 [决策记录](decisions.md)。

gRPC 首次调用或闲置后长时间等待，但 A 记录能正常解析，仍可能卡在额外 DNS TXT 服务配置查询。当前代码明确关闭本项目不使用的服务配置读取；排查既要看库存客户端，也要看 OTLP 底层 gRPC。见 [阶段 3 学习材料](learn/phase-03-observability.md) 的真实过程与台账 P3-18 至 P3-23。

## 下单返回待核对，或订单与销量不相等

`REQUEST_OUTCOME_UNKNOWN` 表示可能已经有库存或订单副作用。保留原幂等键，核对订单、库存、服务日志和后续消息处理；不能换键重试。`REQUEST_IN_PROGRESS` 才是原请求尚未结束。设计见 [阶段 2 学习材料](learn/phase-02-order.md)。

销量暂时落后时，先暂停流量，等 Kafka 消费追平，再运行 `shopctl check`。若长期不相等，核对失败发布、消费者事务和重复消息日志。项目未实现 Outbox，订单提交与消息发布之间有真实缺口；总量相等也可能掩盖局部差错。实测与边界见台账 P2-5、P2-8 和 [销量仓储](../internal/shop/sales/repository.go)。

Redis 因 `noeviction` 达到容量拒绝写入时，下单幂等选择拒绝业务，不能清空幂等键或缩短期限来强行恢复。先查容量和保留记录的原因，已有容量修订见 [阶段 7 汇报](reports/phase-07-reliability.md)。

## 故障恢复了，但 healthy 还失败

恢复动作完成、监控窗口消退、消息追平、持续健康和医生故障开放期关闭，是不同步骤。先看命令指出的具体条件，再查对应 Prometheus 原始值与告警状态。不要降低阈值、删历史指标或跳过冷却。

`healthy` 还会核对活跃商品数、工作者数、QPS、热点、医生创建的索引和运行时连接池变更。小规模演示数据与完整故障基线不相同；若仍在小规模模式，应按操作手册准备实验数据后再验故障，不用伪造基线参数通过检查。见 [健康基线检查](../internal/chaos/verify/baseline.go) 和 [阶段 8 流量守卫](../internal/eval/runner/load.go)。

## 审批等待、执行失败或进程恢复

先看审批单的状态、有效期、具体参数、执行结果和撤销方法。只有保存的批准决定才授权执行；已经过期、拒绝或执行过的单据不能再批。模型提出一组新参数时，需要新单。

进程崩溃时，远端操作可能已经完成。建索引先核对真实索引形状与仍在执行的 DDL；调池核对实例、修订号与操作编号；缓存预热只查仍缺失的项。不要手动把审批状态改回 pending 或直接重发写请求。恢复边界见 [approval execute](../internal/doctor/approval/execute.go)、[断点回放](../internal/doctor/agent/replay.go)。

## 页面断线或看不到新步骤

刷新前先看诊断详情是否已有持久化步骤。SSE 断线后带最后步骤号补发；慢订阅者会被断开，以免拖住诊断。若数据库有步骤、页面没有，检查登录 Cookie、同源、SSE 请求与 `Last-Event-ID`；不要把 JWT 填进 URL。见 [SSE 实现与测试](../internal/doctor/httpapi/stream_test.go)。

## Windows 检查失败

`-race` 需要 gcc 在 PATH 中，并启用 `CGO_ENABLED=1`。只有无外部变量的单元轮通过，不能说集成也通过；集成必须完整注入操作手册列出的专用测试变量，并确认没有 skip。不要将 TEST DSN 指向 shop 或 doctor 业务库，Redis 商品和幂等测试也各用专用测试 DB。

先按 [AGENTS.md](../AGENTS.md) 运行规定检查，固定版本工具以 [CI](../.github/workflows/ci.yml) 为准。格式问题只改本阶段文件；用户保留的未提交修改必须继续排除，不能用全仓自动格式化顺便覆盖。

## 怎样读评分与费用异常

评分协议失败、机制判断错、证据没有拿到是不同问题。正式批次的评分失败保留在台账 P8-191；证据给分尺度分歧保留在 P8-202。不要删失败、改冻结标准或只公布较好分母。交叉评分是模型间复核，未做人工校准；S12 也没有真实暴露攻击文字。

美元按项目用量账本计，即每次请求实际 token 用量乘以 2026-10-29 核对过的官方单价，分档结算，未知请求保留预留。平台账单因 Key 与其他用途共用，无法单独核对；美元与人民币不能相加。DeepSeek 余额差仍未归因，见台账 P8-197。任何排查需要真实模型调用或花钱时先停下取得用户同意。当前展示与检查保持 Mock。
