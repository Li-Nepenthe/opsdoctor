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

阶段 9 的 Demo 使用独立项目 `opsdoctor-otel-demo`，专用医生使用 `opsdoctor-phase09-doctor`；部署组合见[第二系统说明](../deploy/opentelemetry/README.md)。专用医生同时登记 shop 与 otel-demo，连接原 doctor-mysql。原 doctor 容器保留并停止，两套靶场仍串行运行，避免端口、内存和调度冲突。

### 验收后保留了什么，之后怎样由用户删除

前后对照确认原 19 个 OpsDoctor 容器与 2 个 QQ 容器的 ID、挂载和状态不变，原卷删除 0；`opsdoctor-fresh` 的 19 个容器已 stop，9 个卷仍保留，见台账 P10-8 与 [完整对照](reports/phase-10-docker-preservation.json)。默认构建更新了全局 Go 镜像标签，旧容器没有因此重建；Compose 项目名只隔离项目资源，不隔离共用镜像标签。

如果用户以后确认不再需要测试数据，可自行按以下步骤清理。本节是供用户使用的人工操作说明，代理不执行删除。

1. 先确定要弃用的**完整项目名**，例如 `opsdoctor-fresh`、`opsdoctor-fresh-final`；其他 `opsdoctor-cold-*` 名称只作为候选，必须逐个对照 [原版逐轮记录](reports/phase-10-startup-reproduction.json)、[根因复现](reports/phase-10-startup-root-cause.json) 与 [修复验收](reports/phase-10-startup-fix-validation.json) 的完整 `project` 字段。需要的测试结果或数据先按自己的备份流程导出，不能按 `opsdoctor` 前缀批量删除。
2. 在 Docker Desktop 的 Containers 视图中按该精确 Compose 项目筛选，展开后核对容器名称、项目标签与挂载。确认选中的是已停止的测试项目，再由用户手动删除这一个项目的容器。
3. 在 Volumes 视图逐个核对该项目的卷名和使用关系。若界面难以分辨，先运行下方只读命令核对项目标签；只删除这个确切测试项目所属且已无容器使用的卷。
4. 原项目 `opsdoctor`、卷 `opsdoctor_doctor-mysql-data` 及 `qq-astrbot`、`qq-napcat` 均排除。每处理完一个测试项目重新列一次容器与卷，再决定是否处理下一项目。

此外，静态配置验证使用了独立容器 `opsdoctor-alloy-config-validate`。它没有数据卷，用户可以单独核对名称、退出状态与挂载后手动删除这个容器，不需要删除任何卷；不要因名字以 `opsdoctor` 开头就连带选择原项目。

```powershell
docker ps -a --filter 'label=com.docker.compose.project=opsdoctor-fresh'
docker volume ls --filter 'label=com.docker.compose.project=opsdoctor-fresh'
```

bash 使用相同的只读命令。测试项目名相似时，以精确项目标签和实际挂载为准，不凭列表里的相邻位置选择。

## 启动失败先看哪个证据

| 现象 | 先看什么 | 根据证据处理 |
|---|---|---|
| `port is already allocated` | `docker ps -a` 的端口、当前项目和宿主进程 | 先停回占用同端口的那一套环境；保留其卷。不要临时换端口后忘记更新接入配置 |
| `order-worker exited (1)` | 同一次启动的 worker、Kafka、shop-mysql 日志及容器状态 | 区分迁移、主题初始化、消费者启动失败；保留首个错误，不以加重启策略代替定位 |
| MySQL 第一次初始化失败 | `shop-mysql` / `doctor-mysql` 的入口日志 | 看失败的初始化脚本和权限；已有卷不会重新执行初始化脚本。不要为了让脚本再跑而删业务卷 |
| MySQL `Access denied` | 发生错误的是业务、只读、DDL 还是 doctor 连接 | 核对对应环境变量名和授权脚本；独立账号不可互换，避免把业务密码交给医生 |
| 容器 running 但 unhealthy | 该服务 healthcheck、管理端口日志、依赖健康 | running 只代表进程存在；逐项确认真正依赖，不能只增加等待时间 |
| Docker API 连接失败 | Docker Desktop 状态、当前 Docker context、命令权限 | 先恢复 Docker 引擎访问。引擎不可达时，应用日志也不足以解释业务根因 |

阶段 8 的 [CI 41447608555](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/41447608555) 曾出现空环境 worker 退出，当时只保留 MySQL 日志，无法确认当时的具体错误，历史失败保留在台账 P8-207。阶段 10 用原版 `f851460` 与独立新卷先做了 10 轮，均未复现启动退出；最小环境 5/6 轮在主动 stop 后因未启动 Jaeger 的 traces export timeout 退出 1，属于另一个停机现象，见台账 P10-6 与 [早期复现记录](reports/phase-10-startup-reproduction.json)。

随后用 Kafka 内部分区 500、CPU 0.25 的临时放大条件复现了真实启动缺陷：worker 的 Fetch 收到 Kafka 错误 16 就退出，此时同一 Reader 后台仍在入组，Kafka 之后才完成内部协调元数据加载。MySQL 和 Kafka 仍运行，没有 OOM；Compose up 返回 0，后续观察才捕获 worker 退出，见台账 P10-10 与 [根因日志](reports/phase-10-startup-root-cause.json)。参数不同于原 CI，不能据此证明原 CI 当时也一定是错误 16。

修复 `364a889` 只在取到消息之前对协调器错误 14、15、16 最多重取三次，复用原 Reader，退避可取消，不给无订单时的正常 Fetch 添加短 deadline。消息解析、SQL、提交位点及永久鉴权错误不重试。同一放大条件下，最终版出现错误 16 后记录 retry 1 / 250ms，随后同一 worker 成功提交新订单；默认配置也连续 5/5 全新卷真实消费通过，见台账 P10-11、P10-13 和 [前后对照](reports/phase-10-startup-fix-validation.json)。这证明了已复现缺陷的修复，历史 CI 的具体错误仍按未知保留。

修复前 `5806a7e` 的 [CI 41458417476](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/41458417476) 也曾全通过，首次含镜像拉取、构建和启动 337.594 秒，随后 5/5 独立新卷真实消费成功，见台账 P10-9、[CI 记录](reports/phase-10-ci-first.json)。它是保留的旧成功轮，不拿来替代修复 `364a889` 的对照证据。

启动链路先读 [Compose](../deploy/docker-compose.yml) 中 Kafka 的健康条件，再读 [order-worker 组装](../internal/shop/app/orderworker/app.go) 中数据库迁移、主题确保与消费者创建的顺序。Kafka 接受端口连接、能够返回元数据、主题已经可消费，是不同的就绪条件。定位时要保留 Kafka 与 worker 的同一时间窗口日志，不能只凭最终 unhealthy 结论推断。

worker 的健康探针目前只 Ping 数据库，不能证明 Kafka 连接或消息消费。台账 P10-10 的 up 成功后退出已有直接证据，因此健康后还要观察并用真实订单核对消费者是否推进销量；CI 已增加检查，见 [工作流](../.github/workflows/ci.yml)。不要仅根据容器 healthy 就排除消费者问题。

### Demo 的 payment 监听了端口，健康探测仍超时

先比较 payment 业务日志、OOM 状态和健康探测的实际命令。阶段 9 中，端口已经监听、未观察到 OOM，但原 Node TCP 探测会先加载镜像中的自动遥测模块。原命令独立执行可以成功，却超过原超时，见台账 P9-4 与[健康探测证据](reports/phase-09-demo-healthcheck.json)。

本机[覆盖配置](../deploy/opentelemetry/compose.phase09.yaml)只把相同探测的超时放宽到 20 秒，保留业务、重试次数和原始失败。它是经证据确认后的基础设施修正，不是所有 unhealthy 都直接增加等待；也不能据此推断请求路径或日志管线完整。

## README 时间目标的实测范围

`5806a7e` 从远端 git clone 开始，到完成 Mock 演示共 502.941 秒，首次就绪 135.823 秒。首次走查包含浏览器初始化、一次定位器失败与接口复核，用了 189.890 秒；再次按已验证脚本排练为 152.167 秒。两次记录一起保留，见台账 P10-7 与 [计时原始证据](reports/phase-10-fresh-demo.json)。

本机镜像、Go 模块与构建缓存存在，未清空全部 Docker，因此首次下载时间另由台账 P10-9 的 CI 新 runner 补充。网络、机器与缓存不同会影响时间；Mock 演示只证明流程跑通，不证明模型诊断准确率。

## 首次空库没有商品，或只读工具报表不存在

首次启动先暂停流量，等服务迁移完成，再按 README 运行造数与精确授权。`shopctl seed --small` 适合免费页面演示和 CI，小规模数据不能替代故障性能验收；完整故障实验按操作手册的正式基线准备。造数发现已有数据会跳过，不会把旧数据重置。

只有数据库健康还不够，业务表由应用迁移创建。给具体表授权的脚本要等迁移后执行，见 [操作手册](operations.md) 的首次启动步骤与 [MySQL 初始化说明](../deploy/mysql/init/README.md)。授权不足时先核对授权，不改用 root 或扩大权限来“解决”。

## 医生启动了，但没有诊断

先登录页面看运行状态，确认 `LLM_PROVIDER=mock`，再手动创建症状说明。Mock 只演示调用、审计和报告流程，页面必须出现“Mock 模式，不是真实诊断”；不能据 Mock 报告评价根因能力。真实效果查看 [阶段 8 冻结结果](reports/phase-08-formal-results.md)，本阶段不调用付费接口。

如果是自动告警没有建任务，按顺序看 Prometheus 告警状态、Alertmanager 通知错误、目标系统名与 `SHOP_ALERT_TOKEN` 的配置、医生告警接收日志。令牌只在环境变量中核对，不复制到命令行或文档。

有通知但没有新任务，可能是同一故障组被归并、还在恢复观察期、评测模式只捕获通知、排队超过有效期，或全局费用守卫阻止真实任务。查看目标系统的 `fault.open`、`fault.recovery_until`、诊断状态与运行状态；不要连续创建真实任务来试。去重设计见 [故障开放期](../internal/doctor/store/faults.go)，调度原因见 [scheduler](../internal/doctor/run/scheduler.go)。

## 工具不可用、结果为空或被截断

先看目标系统的工具菜单。未配置的数据源会降级为不可用工具，和“后端配置了但查询失败”不同。当前支持 Loki 与 OpenSearch 两个日志实现；OpenSearch 需要单独的 `opensearch` 配置，不能只把 URL 填进 `loki`。同时配置两个日志后端会报错。

OpenSearch 先核对指定索引、实际 mapping 和一条 `_source`，再核对[接入字段](../deploy/doctor/targets/otel-demo.yaml)。本次精确匹配使用 `resource.service.name.keyword`、`severity.text.keyword` 与 `traceId.keyword`，时间为 `observedTimestamp`，正文为 `body`，见[实际字段证据](reports/phase-09-opensearch-schema.json)。查询用的 `.keyword` 不一定存在于 `_source`，resource 也可能使用带点号的完整键；适配器的[字段读取](../internal/doctor/source/opensearch/response.go)专门处理这两种形状。

真实 OpenSearch 按服务及服务加关键词查询已通过（台账 P9-3）。单元测试通过只能证明模拟条件下的行为；真实查询成功也只证明该范围存在可读数据，不能推出全部服务的日志都已采集。

工具结果为空时先核对服务名、时间范围和实际后端是否有数据，再核对过滤条件。日志查询默认围绕故障时间，过去故障可能进入回看窗口；报告必须区分历史与当前。S12 已说明这种限制：攻击文字未进入医生实际看到的工具结果，不能把空结果当成防注入成功。

结果含截断标记时，缩小时间窗、服务或关键词再查。不要扩大工具硬上限来完成演示；它限制故障期资源消耗和模型上下文。实现见 [工具注册表](../internal/doctor/tools/registry.go)、[查询窗口](../internal/doctor/tools/window.go)。

## 有 trace ID，却在 Jaeger 查不到

按顺序确认请求是否被采样、SDK 是否导出成功、Jaeger 是否重启丢失内存数据、搜索时间范围是否正确。查不到不等于请求没发生。消息消费端还要核对 Kafka header 是否携带 traceparent，以及消费者是否提取。

Grafana 的 Jaeger 数据源健康失败时看具体 HTTP 路径。项目固定版本存在经过验证的兼容性例外，不能随意把镜像换成最新版；阶段 3 的失败与修复见台账 P3-9、P3-11 及 [决策记录](decisions.md)。

gRPC 首次调用或闲置后长时间等待，但 A 记录能正常解析，仍可能卡在额外 DNS TXT 服务配置查询。当前代码明确关闭本项目不使用的服务配置读取；排查既要看库存客户端，也要看 OTLP 底层 gRPC。见 [阶段 3 学习材料](learn/phase-03-observability.md) 的真实过程与台账 P3-18 至 P3-23。

## 独立项目有业务日志，Loki 却查不到

先区分应用没有输出、Alloy 没发现容器、过滤规则丢弃与 Loki 写入失败。阶段 10 发现 Alloy 的 Docker 项目过滤原先写死 `opsdoctor`：即使用 `-p` 隔离了容器和卷，采集器仍不会选择新项目。

`c246734` 改为将最终 `COMPOSE_PROJECT_NAME` 注入 Alloy，由 [采集配置](../deploy/alloy/config.alloy) 按实际项目标签发现容器。配置验证与最终新克隆的实际采集已通过：唯一订单trace覆盖订单、库存和消费者三个服务，见台账P10-17及[证据](reports/phase-10-log-isolation.json)。排查时既看 `compose logs`，也要按同一 trace ID 查询 Loki 中订单、库存和消费者的真实日志，不能只看 Alloy 进程正常。

项目名也不隔离全局镜像标签。构建可能更新共用标签，但已创建容器仍指向原镜像 ID；原环境保护要分别核对容器身份、挂载、状态和标签变化，见台账 P10-8。不要把“项目名不同”直接当作日志过滤和镜像都已经隔离的证明。

## Demo 有日志，但 Collector 报映射冲突

若 Collector 的 OpenSearch exporter 出现 `mapper_parsing_exception` 和 `Dropping data`，先保留同一时段日志、索引 mapping 和已有服务分布。本次 cart 启动日志的 `attributes.http` 端口文本先被映射为 text，随后 frontend 的 `http.request.method` 等带点号属性要求它成为 object，引发写入拒收。最终修复前可见窗口共有 316 个 OpenSearch 最终丢弃事件，`dropped_items` 报告和为 6209（台账 P9-13）。这是受影响混合请求的条目报告和，批量写入可能部分成功，不能当成唯一丢失 6209 条或按服务分摊。原始证据和五个场景的取证影响见[日志修复专项报告](reports/phase-09-logging-fix.md)与[最终量化](reports/phase-09-log-impact-final.json)。

这属于采集写入链路缺失，与医生查询已有日志失败不同。已有日志能查到、各服务名出现在索引里，都不能证明失败记录没有丢失；查不到某条错误也不能据此认定业务没有发生。

本次只在 [Collector extras 配置](../deploy/opentelemetry/otelcol-config-extras.phase09.yaml)增加一条日志属性重命名：`http.` 前缀改为 `http_`，例如 `http.request.method` 变为 `http_request.method`。既有 scalar `http`、属性值和正文保留；上游原有两条 scope 规则也保留。启用前确认目标命名空间为空，配置 validate 退出码为 0，医生五个字段的完整 mapping 前后 5/5 相同，原索引 UUID 未变（P9-15）。没有删除或重建索引、数据卷；历史缺口不回填。固定版本外的同名字段碰撞仍需重新检查，不能把这条配置当作任意部署的通用修复。

修复后首个固定窗口为 UTC `2026-11-03T05:04:46.894786114Z` 至 `05:14:47.000000000Z`，共 600.105213886 秒：最终 `Dropping data` 事件、OpenSearch 日志 `dropped_items` 报告和、mapping 错误均为 0，17/17 个预期 OTLP 日志源都有窗口内记录（P9-14）。`image-provider` 官方标明 `logs=False`，零日志单列例外；email 的 9 条日志中有 1 条精确匹配合成测试标记，不能当作真实结账或自然流量覆盖。逐服务原始响应见[固定窗口汇总](reports/phase-09-log-fix-after.json)与[服务查询](reports/phase-09-log-fix-services.json)。

同窗另有 10 条 Jaeger traces `Will retry`，不能写“整个 Collector 零错误”。最终丢弃报告为零也不证明端到端没有静默丢失。当前修复和固定窗口证据仍等用户转交 Claude 审核，付费诊断保持暂停；不以诊断探测绕过审核，也不调整医生提示、工具、手册或评分标准。

## 下单返回待核对，或订单与销量不相等

`REQUEST_OUTCOME_UNKNOWN` 表示可能已经有库存或订单副作用。保留原幂等键，核对订单、库存、服务日志和后续消息处理；不能换键重试。`REQUEST_IN_PROGRESS` 才是原请求尚未结束。设计见 [阶段 2 学习材料](learn/phase-02-order.md)。

销量暂时落后时，先暂停流量，等 Kafka 消费追平，再运行 `shopctl check`。若长期不相等，核对失败发布、消费者事务和重复消息日志。项目未实现 Outbox，订单提交与消息发布之间有真实缺口；总量相等也可能掩盖局部差错。实测与边界见台账 P2-5、P2-8 和 [销量仓储](../internal/shop/sales/repository.go)。

Redis 因 `noeviction` 达到容量拒绝写入时，下单幂等选择拒绝业务，不能清空幂等键或缩短期限来强行恢复。先查容量和保留记录的原因，已有容量修订见 [阶段 7 汇报](reports/phase-07-reliability.md)。

## 故障恢复了，但 healthy 还失败

恢复动作完成、监控窗口消退、消息追平、持续健康和医生故障开放期关闭，是不同步骤。先看命令指出的具体条件，再查对应 Prometheus 原始值与告警状态。不要降低阈值、删历史指标或跳过冷却。

`healthy` 还会核对活跃商品数、工作者数、QPS、热点、医生创建的索引和运行时连接池变更。小规模演示数据与完整故障基线不相同；若仍在小规模模式，应按操作手册准备实验数据后再验故障，不用伪造基线参数通过检查。见 [健康基线检查](../internal/chaos/verify/baseline.go) 和 [阶段 8 流量守卫](../internal/eval/runner/load.go)。

### Demo 的 increase 查询没有样本

先看原始计数器的采样时间间隔，而不是把空结果补成健康。阶段 9 实测 span 计数指标每 60 秒一个样本，原 `[1m]` 窗口可能不足以计算 increase。场景验证已改为 `[2m]`，五个健康条件均返回样本，见台账 P9-7、[原检查](reports/phase-09-baseline-verified.json)和[两分钟窗口检查](reports/phase-09-baseline-two-minute.json)。这只说明验证查询可用，尚未证明故障注入、诊断或恢复流程已经通过。

比较表达式没有使用 `bool` 时，不成立的样本会被过滤掉，成立的 `错误增量 == 0` 则可以保留值为零的样本。因此不能把“返回零”直接当成条件失败，也不能把“没有样本”直接当成零错误。评测器[查询判断](../internal/eval/runner/generalization_scenario.go)按有限非空样本判断条件成立，健康条件还要求存在实际调用。

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

美元按项目用量账本计，即每次请求实际 token 用量乘以 2026-10-29 核对过的官方单价，分档结算，未知请求保留预留。平台账单因 Key 与其他用途共用，无法单独核对；美元与人民币不能相加。DeepSeek 余额差仍未归因，见台账 P8-197。展示、单元测试和 CI 保持 Mock；真实调用只执行用户明确授权的范围。阶段 9 已有独立美元与人民币账户授权，日志采集配置也已修复并完成首十分钟观察（P9-13、P9-14、P9-15）；当前仍等 Claude 审核采集修复后再付费，不能用诊断探测绕过暂停。

最终代码c246734的独立新克隆共304.934秒完成演示，页面脚本102.415秒，原较慢记录继续保留（P10-15）。本机最终保留20个测试项目、138个测试卷，均已停回；精确名称和卷的对应关系见[最终资源清单](reports/phase-10-resource-final.json)（P10-16）。人工清理前按上文逐项核对项目标签；清单之外的原opsdoctor及QQ资源不要选择。
