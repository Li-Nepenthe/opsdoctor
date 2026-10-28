# 设计决定

## 2026-10-26：医生接管告警与命令行入口

先检查了临时 alertsink 的运行时依赖：Compose 的 Alertmanager depends_on、Alertmanager 的推送 URL、Prometheus 的采集目标，以及 Alloy 的日志服务过滤。README 也有查看 alertsink 日志的命令。阶段 5 把这四处运行时接线都换成 doctor；默认环境不再运行 alertsink。保留 cmd/alertsink 和 platform/alertsink 的历史示例代码，让阶段 3 学习材料中的源码链接仍可阅读；它不参与当前接收链路，令牌变量也改用 SHOP_ALERT_TOKEN。

告警路由为 /api/v1/targets/shop/alerts。系统在解析正文之前先验证自己的令牌。任务去重使用“系统名加分组键”的摘要唯一约束；任务和告警快照在同一事务内保存，提交成功才响应。重复通知更新快照；没有现有任务的单独恢复通知不创建任务。阶段 5 不调度 pending 任务，也不在告警恢复时删除它。后续阶段把任务转为终态时，需要清空 active_group，才允许同组开始新一轮诊断。

doctorctl 通过已登录的医生 HTTP 接口调用工具，增加 POST /api/v1/targets/{target}/tools/{tool}/run 这个手动取证入口。它使用服务器的同一个工具注册表，不在 CLI 进程里重建适配器；因此同时打开几个终端，仍共用服务器的三连接 MySQL 池。工具 JSON 由注册表统一校验，单次十秒，完整 Result JSON 不超过四千字节。DOCTOR_URL 与 DOCTOR_TOKEN 来自操作者环境，不是工具参数。

医生自己的库使用独立 doctor-mysql，512 MiB 上限、64 MiB buffer pool、最多 30 个连接，关闭这个实例未使用的 performance_schema。医生进程上限 256 MiB。它可以写自己的任务库，但只拿 shop 的只读 DSN 和 Redis URL。运行时只挂 targets 目录，不挂整个仓库或 Docker socket。首次启动从环境初始化一个登录用户；重复启动不会覆盖已有邮箱的密码。doctor 自己的池上限 10，靶场只读池上限 3，两者不能混为一个数字。

## 2026-10-26：Jaeger 2.20.0 查询契约与只读 MySQL 列权限

先在当前固定的 Jaeger 2.20.0 实测 /api/services、/api/traces?service=order-service…、/api/traces/{traceID} 均返回 200。医生选择 /api/traces 系列 JSON 查询接口，与已验证的 Grafana 接线相同。它是 Jaeger 的内部查询 API，不能宣称为稳定的跨版本标准接口；依据：[Jaeger 2.20 API 说明](https://www.jaegertracing.io/docs/2.20/architecture/apis/)。

从真实订单 trace 取了一份 24 个 span、跨三个服务的搜索和详情响应作为 httptest 夹具。只把 db.connection_string 替换为省略标记，避免保留连接细节；适配器输出本来也不允许该标签。契约测试覆盖查询路径、微秒时间、服务名、父子关系和 SQL 模板。Jaeger 升级时，除 Grafana 回归外，必须一起回归这个医生适配器；原来 2.22.0 与 Grafana 的版本例外继续有效。

MySQL 的锁等待直接查询 performance_schema.data_lock_waits 与 data_locks，不绕到需要额外权限的 sys 视图。账号只授这几张监控表实际用到的列，语句摘要不授 QUERY_SAMPLE_TEXT，锁表不授 LOCK_DATA；业务五张表只授 SELECT，让 EXPLAIN 可用。没有工具执行输入 SELECT 或返回业务行。MySQL 和 Redis 是原生协议，适配器单元测试使用 database/sql 假驱动和 RESP 小服务器；HTTP 数据源使用 httptest，真实账号权限另用集成测试证明。


## 2026-10-26：Redis 补认证，复验固定在 1 万商品基线

阶段 5 检查发现 Redis 的 default 用户从阶段 3 起一直是 `on nopass +@all`。仅增加一个医生只读账号还不够：匿名连接可以绕过这道边界。用户明确批准给 default 加密码，保留原业务命令权限，医生使用独立的 diag_ro 密码。

新增 `deploy/redis/start-with-acl.sh`。脚本从 REDIS_PASSWORD 和 SHOP_REDIS_RO_PASSWORD 读值，用 umask 077 在内存目录生成 ACL 文件。文件存 SHA-256 密码哈希，目录 0700、文件 0600，归 redis 用户所有。redis-server 参数只含文件路径；redis-cli 健康检查使用 REDISCLI_AUTH，exporter 使用官方支持的 REDIS_PASSWORD 环境变量。订单服务、chaos 和业务 Redis 集成测试全部同步。新测试密码变量缺失会明确 Skip，给了错误密码则测试失败。依据：[redis_exporter 参数说明](https://github.com/oliver006/redis_exporter#command-line-flags)。

医生 ACL 先撤掉全部命令，再放行 HELLO、PING、INFO、SLOWLOG GET/LEN。不放行 key 访问，不开放 SLOWLOG RESET。慢日志原文可能带业务键和值，适配器输出只保留命令名、时间、耗时和参数个数。Redis 原生协议用假服务器测试；真实权限另跑集成测试。清库等负向测试先做全部 ACL DRYRUN，只要一条没有明确拒绝就停止，防止错误配置真的删数据。

重建后的压测范围回到了代码默认的 50 万商品，200 QPS 下正常回源比例约 44%，仍会触发 CacheLoadsHigh；阶段 4 的 P4-16 已出现过参数变化问题。用户决定本阶段不改告警和代码默认值，按 1 万商品、200 QPS、64 个工作者复验，告警适用范围另行修复。三项明确写入本机忽略的 .env，README 在启动前提示核对参数。数据见 P5-7、P5-8；不能把缩小工作集说成修好了告警。


<a id="phase-05-startup"></a>

## 2026-10-25：区分启动建立任务与持续泄漏

Claude 的整套环境重启验收中，进程刚启动不到两分钟就触发了 `GoroutinesGrowing`。本轮在旧规则下另测一次冷启动：订单服务增长率峰值为每秒 1.981 个，超过原门槛的最后一个样本在进程启动 64.976 秒处；规则进入过 pending，这轮没有变成 firing。用户此前的误报记录仍然成立，不能因为这一轮没有复现 firing 就抹掉它。

规则增加“当前进程运行超过 90 秒”的前提，原有增长率门槛和一分钟持续判断不变。启动时间与 goroutine 数按 job、instance、service 匹配，避免取到另一个实例的年龄。这不是宣称启动阶段一定没有泄漏，而是暂时不把建立连接和后台任务当成泄漏信号。持续泄漏仍须在三分钟内发现：真实 S09 在注入开始后 161.078 秒触发，随后恢复正常版本。

整套 `compose stop/start` 后收集到 476.274 秒、95 次样本，其中停止期间 1 次后端不可用，其他样本没有任何 pending 或 firing 告警。该轮增长率本身也未超过 1，因此还补五组合成时间线覆盖保护条件；不能只凭这次没有误报，就说任何机器的启动都已验证。下单延迟规则保持不变，MySQL 刚启动时缓存数据页不足导致的实际变慢仍应被观察到。

重试预算测试改用标准库的虚拟时钟，没有修改生产代码或放宽断言。集成测试说明同时补齐七个变量，增加跳过数检查；测试库虽然隔离数据，仍共享数据库进程，测试压力不能当作业务故障。医生接收过的测试告警会留下 pending 任务，暂停通知不会删除既有记录。

<a id="phase-04-cache-alert"></a>

## 2026-10-25：缓存告警同时看回源比例和此前流量

Claude 验收发现，停流让商品缓存自然到期后，恢复访问就会触发原来的 CacheLoadsHigh。原规则只判断回源是否超过每秒 50 次，没有持续时间。此前故障对照通过，说明它能发现实验中的回源增加，却不能证明它会放过正常冷启动。用户要求返工，并明确要求 S04、S05 仍在三分钟内报警。

按用户要求，把判断改为最近一分钟的商品回源数除以同一窗口的商品 HTTP 请求数。分母只用 GET /products/:id，避免把缓存内部的第二次检查当成新的请求。每条计数先计算速率，再按 service 汇总，这样服务重启的计数归零不会被当成负增长。

本次采用的门槛为 35%，并要求当前和两分钟前的商品请求速率都超过每秒 20 次，全部条件持续 30 秒。两分钟前的请求量提供了回源发生前的背景：长时间停流后可以先填充，持续有流量时突然大量回源则需要观察。原有两轮实际样本中，S04 的条件各持续 50 秒，S05 各持续 40 秒，因此选择三十秒观察期；历史回放只是校准依据，仍须做本轮实机验收。

放弃单独提高“每秒回源次数”门槛，因为它仍随总流量和活跃商品数变化。也没有只提高比例门槛：S05 的历史峰值约 51%–53%，冷缓存恢复可能比它更高，单个比例不能区分两者。同键并发回源计数、singleflight 配置和实验状态都不进入告警，保留医生自己调查原因的过程。

这条规则的适用范围仍需写明。当前实验为 200 QPS、10000 个活跃商品、64 个工作者。冷启动初期或每秒不超过 20 次商品请求时，它不判断缓存异常；耗时和错误率告警继续工作。更大工作集如果需要更久才能填充，应重新测量，而不是直接照搬两分钟。告警描述的是回源负担升高，不能单凭它断言发生了击穿或雪崩。

实机验证已完成：暂停超过十一分钟后恢复，连续 1234.171 秒无告警；随后 S04、S05 各两次完整流程通过。正常到期比例峰值 20.261%，S04 两轮为 90.384%–90.595%，S05 为 47.048%–49.441%；所有触发与恢复均小于三分钟。额外完整 selftest 又通过 11/11，S04、S05 各完成第三次流程。具体数字和失败记录见 P4-28 至 P4-39。

规则测试使用五组人工时间线，并接入固定版本 CI；这些数据只验证判断逻辑。真实停流、持续健康与两个场景的结果单独保留在 [notes](notes.md#phase-04-cache-rework) 和[数据台账](数据台账.md)。旧阈值对应的 P4-13、P4-16、P4-21 继续保留为历史证据。

## 2026-10-25：空库先建实验账号，迁移完成后再授表权限

首次阶段 4 CI 中，MySQL 初始化脚本运行时业务表尚未创建，针对 stock 的授权报表不存在。业务表由 Go 服务启动迁移创建，服务又必须等 MySQL 可用，不能把表级授权当作 MySQL 首次启动的前提。

调整 005_chaos_user.sh：始终先建独立账号；两张表齐全时才授予原有的 SELECT、ALTER、INDEX。空库先完成数据库启动，服务健康后由操作者或 CI 显式再执行脚本。已有数据卷也沿用这条命令。独立空库与重复授权实测见 P4-26。放弃扩大到 shop.*，也不把业务迁移复制到初始化 SQL 中；医生和业务服务的权限均不变。

## 2026-10-25：扩大长实验所需的 Redis 容量

连续实验保留的幂等键约三十万个，Redis 达到原有 128 MiB 写入上限。noeviction 按原设计拒绝新写入，下单返回 503；S04 已观察到缓存症状，却无法通过恢复健康检查。失败完整保留在 P4-17。

把 Redis 的 maxmemory 改为 512 MiB、容器上限改为 768 MiB，为后续两轮留下容量，并给分配器、连接及 AOF 重写留出额外空间。全套容器限额合计由 6464 增为 6976 MiB，仍在计划的本机内存范围内；限额之和不等于实际占用，结束时另测。

保留 24 小时幂等期限、noeviction 和 AOF，不通过清键或缩短保证期限让测试通过。重建复用原卷，前后都数到 299587 个幂等键。512 MiB 仍是有限容量，不承诺能持续承载一天的 200 QPS；更长实验需估算订单数与单条记录大小，并继续监控。这个调整只扩大现有组件的资源预算，没有新增组件或改变幂等行为。

<a id="phase-04-s04"></a>

## 2026-10-24：S04 只制造有上限的商品查询突发

本条保留当时的注入与对照决定。下面的每秒 50 次告警门槛已经进入[冷缓存返工](#phase-04-cache-alert)，不作为当前规则说明。

原方案把 400 QPS 的商品查询和下单都集中到 SKU 1。第二轮实测中，缓存补好后回源迅速归零，预期缓存告警没有出现；反而是下单争抢同一行库存，出现了 RowLockWaiting。这无法把 S04 与 S08 区分开。失败轮次保留在 notes 和数据台账 P4-8。

用户批准改成短时并发查询，模拟秒杀刚开始时一批用户同时打开热门商品详情。新增的 `/burst` 只发商品查询，复用 loadgen 已有的固定工作者。单批大小不超过工作者数，硬上限仍为 256；上一批未结束时拒绝叠加。普通请求与突发请求共用同一组工作者，不能各算各的上限。接收成功不代表执行成功，控制接口分别显示完成、失败和未发出的数量。

同一商品被多个请求同时查数据库的计数只用来帮助调查，不能用作告警条件。告警仍描述用户能观察到的症状：实际回源或数据库查询速率超过健康对照，或者商品查询 P99 升高。阈值通过两组实测确定，不能为了让场景通过而直接对“关闭 singleflight”告警。实验同时检查行锁告警；若主键查询便宜、影响有限，材料中就保留这个限制。

初步对照中，单个商品的 64 次查询只增加了很少的回源；100 个热点各 64 次虽能出现缓存症状，却让正常组的库存查询也出现连接池等待。最终候选采用 400 个热门商品，每个只经历一次失效，每批 16 个查询、批间隔至少 50ms，共 6400 次查询，约二十秒。可以把它理解成一批促销商品陆续开放，各自页面迎来一小批同时访问的用户。它占已有五十万商品的 0.08%，没有持续删除同一个键，也没有给 SQL 增加人为等待。批次执行器最多接受 500 批、每批最多 64 次；仍受整个场景三分钟时限约束。

16 并发的初步对照：正常组回源 400 次、P99 16ms；关闭合并回源后回源 6316 次、P99 20ms。分钟回源速率分别约 14.676 / 121.549 次每秒，所以保留每秒 50 次的 CacheLoadsHigh 门槛。正常组从开始到结束后约一分钟的历史 ALERTS 中没有 firing 告警，两组行锁速率均为零。普通流量在这段短实验期间暂停，再恢复原基线；通用 `--qps`、`--sku` 不允许覆盖这份突发场景，防止把暂停或只查询的边界改掉。

2026-10-25 正式容器对照使用同一镜像和同一负载，每组 6400 次商品查询。开启合并时回源 400 次，关闭时回源 6295 次；商品 SQL 速率为 19.365 / 295.521 次每秒，HTTP 直方图估计 P99 为 47.794 / 85.799ms。两组行锁等待时间增量都是零。后续 80 秒的观察中，正常组没有 firing 告警，故障组只有 CacheLoadsHigh。商品回源是便宜的主键查询，这次没有把数据库打垮；回源越昂贵，重复工作才越容易造成更严重的后果。原始口径与数字保留在 P4-13。

门槛按 200 QPS、10000 个活跃商品、64 个工作者校准，数据库仍是五十万商品。重建 loadgen 时遗漏活跃商品参数，默认五十万工作集曾让正常回源达到约 85.408 次每秒，健康检查因此失败，记录在 P4-16。恢复原实验工作集后才继续验收。没有改应用默认值，也没有把这一失败写成业务故障；换工作集或 QPS 必须重新核对静态门槛。最终连续两轮 S04 均只触发 CacheLoadsHigh，没有 RowLockWaiting，见 P4-21。

## 2026-10-24：实验记录与恢复边界

场景的 inject、restore 使用动作列表，支持同一实验中的配置、网络和流量顺序。symptom 增加相关告警名列表，自动验收需要业务症状与其中一条 firing 告警，不能拿不相关的服务重启告警充数。新增 internal/chaos/app 只做命令与流程装配；没有新增计划之外的业务领域或组件。

注入前先给原镜像增加独立的本地保留标签，再把场景、标签、原镜像内容 ID、公开配置和流量设置写到宿主机 `.opsdoctor-chaos.json`。独占创建防止两轮叠加，落盘后才执行动作。恢复前核对保留标签的内容 ID，禁止拉取或重新构建。首轮 S03 实测证明，只保存 ID 不足以保留本地镜像：原标签被覆盖、容器重建后，原 ID 已不可使用。保留标签解决的是内容仍可取得的问题，内容 ID 则用于核对取得的是否为原版本。记录不含密码；它包含参考答案，因此 Git 与 Docker 构建都排除它，业务和医生容器都不挂载它。restore-all 只恢复这份活动记录，不重置别的项目或其他人的网络规则。恢复成功才删除记录。

TTL 表示缓存还能保留多久。S05 只给 product: 范围中的已有键设置同一个到期时刻，模拟一批缓存共同失效，不清库、不触碰幂等记录。被删除或过期的缓存值无法原样还原，恢复配置后由正常请求重新填充。库存补充仍只允许在实验前显式执行，selftest 内不会悄悄改库存。

<a id="phase-04-attempt-timeout"></a>

## 2026-10-24：区分查询的一次等待与整次调用时限

原 GetStock 所有尝试共享 800ms 截止时间。首次调用若因延迟用完这段时间，提高重试次数也不会再次发出请求，无法形成 S13 要验证的重试风暴。用户明确批准新增 INVENTORY_GETSTOCK_ATTEMPT_TIMEOUT：默认 0，保留原行为；显式设置时不得超过总时限。查询重试上限扩展到通用重试器已有的 10 次硬上限，默认仍为 2 次。Deduct 和 Release 不经过重试器，也不使用单次查询预算。

新参数随生效配置写进启动日志。测试比较默认共享截止时间、短单次预算下的实际多次查询，并检查全部允许的重试次数都不会重复扣减。S13 使用较短单次预算、较高重试上限、关闭退避和熔断，再施加真实网络延迟。证据必须同时包含 rpc_client_retries_total 激增与库存服务实际请求量超过 HTTP 入口，不能只根据配置就宣称重试风暴成立。

## 2026-10-24：给已提交订单单独留出消息确认时间

P3-25 的 53 条告警没有保存底层错误和等待时长，无法再确定每次最初为什么变慢。固定版本 kafka-go v0.4.51 的 Writer 会把批次交给后台发送；调用方的 context 到期后可以先返回，但后台批次仍可能成功。这解释了“调用方未收到确认，消息后来仍被消费”，不证明当时发生了哪一种网络或 broker 故障。源码依据：[WriteMessages 的取消语义](https://github.com/segmentio/kafka-go/blob/v0.4.51/writer.go#L580)、[后台 produce 的独立预算](https://github.com/segmentio/kafka-go/blob/v0.4.51/writer.go#L728)。单元测试保留真实 Writer，只替换 broker 响应，复现先取消等待、后成功确认的顺序。

原订单确认预算为 2 秒，Writer 单次写入允许 3 秒，还可能排队和重试。新增 ORDER_PUBLISH_TIMEOUT，默认 5 秒，上限 10 秒，独立于 Redis 状态收尾与库存补偿的 2 秒预算。幂等处理中时限、HTTP 写出时限和优雅退出预算一并包含这段等待。保留原有 10ms 凑批时间；它已不是 kafka-go 的默认一秒，没有证据需要再缩短。发送仍使用 context.WithoutCancel，保留调用链信息。

五秒是给健康状态下短暂抖动留出的余量，不是送达保证。确认超时仍必须告警，不能把后台“可能成功”写成“已经成功”。现在告警附带错误类别和等待时长，Writer 也记录单次发送失败，供以后区分客户端截止、连接和 broker 问题。调整前后的实测分别保留在 notes 与数据台账，若基线没有复现告警，就不计算虚假的改善比例。

<a id="phase-04-foundation"></a>

## 2026-10-24：阶段 4 的实验基础

- 用户授权合并阶段 3 和进入阶段 4；本阶段只实现 S01–S10、S13，完成后停下来。按新第 3.6 节写说明，保留 Claude 对早期材料的重写。
- 已阅读 [coroot/rca-lab 的 Not giving away the answer](https://github.com/coroot/rca-lab#not-giving-away-the-answer)。业务日志和指标保留真实发布信息；场景答案只在宿主机场景文件中。缺陷构建里的函数使用普通业务名称，不在堆栈里提示根因。
- 新组件选择 [Toxiproxy v2.13.0](https://github.com/Shopify/toxiproxy/releases/tag/v2.13.0)，为查询时官方最新稳定版。调用它的 HTTP API 使用标准库，不额外引入客户端库。[kafka-go v0.4.51](https://github.com/segmentio/kafka-go/releases/tag/v0.4.51) 仍是官方最新稳定版。Jaeger 继续沿用已批准的 2.20.0 例外。
- 长实验会把热门商品卖空，后面的行锁实验就没有足够的成功更新。用户已明确允许补库存，因此新增 `shopctl stock restock --sku ID --quantity N [--count N]`，只在实验前显式执行。它复用正式 gRPC Release，不直接写表。count 默认 1，最多 10000；每个商品单独确认，失败时停止并报告位置。Release 未实现请求去重，所以不能在未查询结果前重跑失败批次。
- Jaeger 从 5000 调整为 30000 条 trace。按 20 条/秒计算约容纳 25 分钟，给十五分钟目标留余量；这只是容量推导，最终还要记录真实流量下的保留时间。容器上限从 512 MiB 增到 1024 MiB，接收限流阈值为 800 MiB，给查询和运行时留空间。保持内存存储，重启仍会清空。

## 2026-10-23：阶段 2 开工

- 用户已核实阶段 1，并明确同意合并、推送 main 和进入阶段 2。main 快进到 `3970868`，随后创建 `phase-02-order`；没有勾选学习验收。
- 本阶段包含计划列出的订单、缓存、消息、压测和服务治理。用户另外明确要求结果待核对计数，因此只接入 `idempotency_requests_total{result="needs_review"}`；其他指标、调用链仍留到阶段 3，不实现 Outbox。
- 文件头按第 3.1 节补完整：用途、单独拆文件的理由、调用顺序、“主要内容”的函数清单，以及阅读建议。注释先讲大白话，再介绍术语。
- 依赖版本通过 `go list -m -json ...@latest` 实际查询：go-redis/v9 v9.23.0、kafka-go v0.4.51、gobreaker/v2 v2.4.0、x/time v0.16.0、x/sync v0.24.0。前四项按计划选型，标准库没有对应 Redis/Kafka 客户端或现成治理组件；singleflight 用 x/sync 的小工具，避免自己维护等待队列。没有另加测试框架。
- 幂等失败边界已按 AGENTS.md 第 6 节向用户提出，用户明确同意并补充独立状态与计数。规范同步到计划书第 5.2.6、5.8.2 节，具体决定见下文。

<a id="phase-02-idempotency"></a>

## 2026-10-23：阶段 2 幂等失败边界

- 原因：一律在失败后删除占位，会让“扣库存成功但响应丢失”等情况重新执行下单，可能重复扣库存。用户确认采用保守处理。
- 决定：确认订单没有写入，并且库存没有扣减或已成功归还时，才允许删除占位。订单已经写入但消息发送失败，返回已创建结果，并记录发送失败告警。
- 扣减或提交结果不确定、补偿失败、幂等结果保存失败时，保留占位并进入“结果待核对”。返回 409 `REQUEST_OUTCOME_UNKNOWN`，与还在执行的 409 `REQUEST_IN_PROGRESS` 区分。
- 每次进入待核对状态，记录订单号、商品 ID 和幂等键。尚无订单号时用 `0` 表示未知。同时增加 `idempotency_requests_total{result="needs_review"}`，不把幂等键或订单号放进指标标签，避免每笔订单都产生一条新的时间序列。
- 计数依赖固定为 `github.com/prometheus/client_golang v1.25.0`，由实际版本查询确认。只提供本次要求的计数，不提前搭建阶段 3 的监控组件。
- 放弃的做法：错误后直接删 key、自动重试 Deduct、增加 Outbox、修改库存 RPC 契约。根治扣减超时需要 Deduct 以请求 ID 去重，按用户要求本阶段不实现，并列入已知限制。

## 2026-10-23：阶段 2 Redis 保存与压测统计

- Redis 现在同时保存缓存和幂等记录。阶段 0 的 `allkeys-lru` 会淘汰幂等凭据，无持久化重启也会丢记录，因此 Compose 改为 `noeviction`、AOF `appendfsync always` 和独立数据卷。内存不足时拒绝新写入，订单返回不可用；不能以删旧凭据换取继续下单。仍是计划内的单个 Redis，不增加组件。
- 幂等记录保存原有效期，不因回放续期。旧请求只能比较并替换自己看到的记录。处理中记录超出“HTTP 预算 + 三次收尾预算 + 一秒余量”后转为待核对，覆盖进程退出或 Redis 故障期间未能写回状态的情况。
- 幂等键绑定解析后的用户、商品和数量。同一键改参数返回 400。没有提供键的请求遵循契约继续允许下单，但不获得请求去重保证；保存窗口过去或 Redis 数据卷丢失同样超出本阶段保证。
- loadgen 采用固定工作者、无积压队列。工作者全忙时记录 dropped，不无限增加 goroutine。统计按实际完成的 HTTP 尝试计算，重试独立计入完成数与错误数；延迟使用一毫秒直方图桶，上界估计 P50/P99，超出六十秒桶范围的样本另外计数。
- 默认 Zipf 参数为 s=1.1、v=100，商品范围由配置决定。前一小组商品明显更热，但不把全部基准订单集中到一件只有一百件库存的商品上；控制接口可通过 hot_sku_id 专门制造单商品热点。QPS=0 暂停，热点编号=0 恢复 Zipf 分布。

## 2026-10-23：阶段 0 的范围与验收环境

- 决定：只实现计划书阶段 0；从 main 的 `3ebd194` 创建 `phase-00-foundation`。不建立空业务包。
- 原因：先让共享基础设施和检查工具可学习、可验证，再按阶段增加业务。
- 放弃的方案：提前搭建库存服务、医生 HTTP 服务或后续阶段目录。
- 环境决定：用户在本次对话明确同意“继续写代码；我来准备 Docker，本机 race 暂由固定版本 Linux CI 验证”。CI 固定为 `ubuntu-24.04`。
- 实测限制：初次检查未找到 Docker 和 C 编译器，CGO_ENABLED=0。Docker 健康状态必须实际验证，不能把未执行当成通过。

## 2026-10-23：复用来源

- 只读来源：`D:\Code\GO项目\ai-backend`，main 提交 `095337690a9a881cba7a8eced3aae950b6016582`。
- 复用 JWT/bcrypt、MySQL 连接池与 UTC 会话、命名锁迁移、HTTP 中间件及 CI 思路。
- 按本项目拆包、补充中文注释；不复制文档问答业务、业务表或领域依赖。

## 2026-10-23：阶段 0 版本锁定

Go 库用 `go list -m -json <模块>@latest` 查询稳定版本后固定；不在 CI 中使用 `@latest`。直接依赖之外的版本由 go.mod/go.sum 记录，不主动引入其功能。

| 组件 | 固定版本 | 原因与放弃的替代方案 |
|---|---|---|
| Go | 1.27.1 | 按计划书和本机版本，不随 CI runner 自动升级 |
| Gin | v1.12.0 | 按技术栈复用旧中间件；不另选 HTTP 框架 |
| go-sql-driver/mysql | v1.10.1 | database/sql 驱动；不使用 ORM |
| golang-jwt/jwt/v5 | v5.3.1 | 复用已验证的 JWT 实现；不自行实现签名协议 |
| golang.org/x/crypto | v0.58.0 | 使用 bcrypt；不自行实现密码哈希 |
| staticcheck / honnef.co/go/tools | v0.8.2 | 查询时的稳定版本；不沿用旧仓库 v0.8.1 |
| MySQL | mysql:8.4.13 | 计划指定 8.4 LTS，选该系列已发布补丁；不切换其他大版本 |
| Redis | redis:8.10.2 | 查询时稳定版本；这里只启用基础缓存实例 |
| Kafka | apache/kafka:4.4.0 | 官方 KRaft 镜像；manifest 已实际查询，无 ZooKeeper |
| actions/checkout | v7.0.1 | GitHub releases/latest 查询结果，固定完整版本 |
| actions/setup-go | v7.0.0 | 同上，Go 版本显式固定为 1.27.1 |
| CI 系统 | ubuntu-24.04 | 按计划固定系统，不用 ubuntu-latest |

版本来源：[MySQL 官方镜像清单](https://github.com/docker-library/official-images/blob/master/library/mysql)、[Redis 发布页](https://github.com/redis/redis/releases)、[Kafka 官方下载目录](https://downloads.apache.org/kafka/)、[Gin 发布页](https://github.com/gin-gonic/gin/releases)、[MySQL 驱动发布页](https://github.com/go-sql-driver/mysql/releases)、[JWT 发布页](https://github.com/golang-jwt/jwt/releases)、[Staticcheck 发布说明](https://staticcheck.dev/changes/)。镜像存在性最终以实际成功拉取为证。

## 2026-10-23：基础设施的实现取舍

- config 注入读取函数，标准库足够；不增加 dotenv/Viper。服务自己的配置结构体留到对应 app 出现时再写。
- logging 使用独立 slog.Logger 和不变的 context 字段，不修改全局 logger；trace/span 仅承载已提供的标识，不提前接入阶段 3。
- HTTP 保留旧代码的请求标识校验和安全恢复，按新契约统一为 `INVALID_ARGUMENT`；不迁入旧业务错误分类。
- 迁移 API 接收 fs.FS，生产调用方可以传 embed.FS。一文件一条 SQL，禁用 multiStatements；不继承旧实现按分号拆 SQL 的做法，因为字符串内也能包含分号。执行前检查历史校验和，命名锁始终使用同一个 sql.Conn；DDL 仍需可重入，不承诺整批回滚。这是阶段 0 迁移基础设施的细化，不新增业务迁移。
- 退出流程拆为 Notify 和 Shutdown，由后续 app 决定如何停止接收请求和工作者；不提前写服务入口。关闭函数必须响应 context，通用层不能强杀任意阻塞代码。
- commentcheck 使用标准生成标记跳过生成代码，外部测试包沿用同目录被测包的 doc.go。archcheck 对可静态识别的文件读取做检查；它不能穷尽运行时拼接、函数别名和跨文件数据流，后续仍须依靠配置与最小权限。
- 仅创建计划书列出的本阶段目录；没有空业务包，也没有新增技术栈组件。

## 2026-10-23：Compose 首次启动修正

- 默认 `.env` 查找未找到仓库根目录文件，因此 README 显式使用 `--env-file .env`。先注入 shell 环境时，计划书原样的 Compose 命令也实测通过。
- Kafka 数据路径使用官方镜像预建且属于 appuser 的 `/var/lib/kafka/data`。首次尝试 `/tmp/kraft-combined-logs` 的空卷属主不匹配，导致启动失败；已修复并保留失败记录。
- MySQL 只初始化空 shop 库和两个隔离测试库；本阶段没有业务表。迁移测试强制校验专用库名，避免清理业务库。
- 用户随后安装并启动 Docker、安装 gcc。本机 CGO 已启用；最终同时跑本机 race 和 Linux CI，不再依赖暂时的替代验收安排。

## 2026-10-23：阶段 1 的范围与版本

- 决定：用户核实阶段 0 返工后，明确授权合并、推送 main，并进入阶段 1；据此从 main 创建 `phase-01-inventory`。学习验收仍留给用户勾选。
- 原因：阶段 1 只交付库存 gRPC、五张业务表迁移、造数和手动调用命令；订单服务、缓存和消息处理留到阶段 2。
- 放弃的方案：提前实现下单、消费历史订单，或给库存扣减添加未经计划定义的幂等字段。
- 协议文件采用第 5.1 节的 `api/proto/inventory/v1/inventory.proto`，第 6 节的 `proto/inventory/v1/inventory.proto` 按协议子目录的简写理解；没有另建顶层 proto 目录。

| 新组件 | 固定版本 | 决定原因 / 放弃的方案 |
|---|---|---|
| google.golang.org/grpc | v1.84.0 | 查询稳定版本后写入 go.mod；不用另一种 RPC 协议 |
| google.golang.org/protobuf | v1.36.12 | 与生成插件一致；不手写协议序列化 |
| buf | v1.75.0 | 按计划统一 lint 和生成；不用人工拼接 protoc 参数 |
| protoc-gen-go | v1.36.12 | 本地固定插件；不使用浮动远程插件 |
| protoc-gen-go-grpc | v1.6.2 | 同上，生成客户端和服务端接口 |
| 构建镜像 | golang:1.27.1-alpine3.24 | 固定 Go 和 Alpine 系列，实际拉取成功 |
| 运行镜像 | alpine:3.24.2 | 非 root 运行，保留 wget 健康检查；不携带编译工具链 |

现有 MySQL、Redis、Kafka 和 CI 版本沿用阶段 0。间接依赖精确版本以 go.mod/go.sum 为准。

## 2026-10-23：库存与造数的边界

- 决定：用带 `available >= quantity` 条件的单条 UPDATE 扣库存，借助 `uk_stock_sku` 定位；更新失败后才查询存在性来选择错误码。
- 原因：先读余额再写余额会留下并发窗口；进程内互斥锁也保护不了多个服务实例。放弃先查后改，不额外引入分布式锁。
- 决定：默认连接池 20 个打开连接、10 个空闲连接、5 分钟连接寿命。这是本机靶场的起始配置，并非压测得出的最优值；MySQL 最大连接数为 100，需要给迁移、CLI 和以后其他服务留余量。
- 决定：seed 每批最多 1000 商品和 4000 订单，五张表在同一事务提交；50 万商品对应 200 万历史订单，同时写入销量和已处理事件基线。
- 原因：中断时不会出现本批只写了部分表，后续消费也不会把历史数据重复计算。放弃逐条提交、全量大事务和向 Kafka 重放历史订单。
- 限制：seed 是独占运行的离线初始化工具，按行数和连续 ID 检查已有基线，不能验证任意业务库的内容一致性。重跑不重置已变动库存；新订单上线后不要将它当成修复工具。
- 决定：库存测试和 seed 测试各用独立数据库，并在清理前严格校验库名。原因是 `go test ./...` 会并行运行不同包，不能让两个测试互相清表。
- 决定：RPC 的 context 一直传到 QueryRowContext/ExecContext；客户端关闭业务重试。超时表示调用方停止等待，不保证 UPDATE 没有提交，也不保证远端 SQL 在同一瞬间终止。

## 2026-10-23：三层说明规范与 archcheck 的精确例外

- 决定：按用户要求，把 main 的 `3aa6c99` 合入阶段 1，再扩展 commentcheck 的文件说明、doc.go 文件清单校验和 `-map` 模式；同时补齐阶段 0、1 的目录与文件说明。
- 原因：目录说明帮助找入口，文件说明解释拆分理由，代码注释解释关键判断；函数注释不再机械罗列“不阻塞、并发安全”。放弃用统一模板替代实际讲解。
- 发现的冲突：第 5.1.8 节要求 `cmd/README.md`，旧 archcheck 把 cmd 下所有非 main.go 文件都判错。
- 用户授权：在对话中明确同意只放行精确路径 `cmd/README.md`，并要求在计划书第 3.4 节写明“cmd/ 目录本身可以放 README.md”。两处已同步。
- 最小调整：只修改 `locationProblem` 的这一条路径判断，新增对应正例，并继续拒绝 `cmd/demo/README.md`、`cmd/demo/doc.go` 和 `cmd/demo/extra.go`。
- 放弃的方案：放宽整个 cmd 子树、取消薄入口约束，或为了绕过工具把必需的目录说明省掉。除此项与用户要求的 commentcheck 扩展，阶段 0 的 Go 文件只改说明；以忽略注释和位置的 AST 比较及 Go 指令逐行比较核验。


## 2026-10-23：阶段 3 的版本与观测口径

- 用户明确验收阶段 2，授权合并并进入阶段 3。main 上的 `623f7c2` 与后来增加的 `fed64bf` 都保留，合并后推送，再创建 `phase-03-observability`。不勾学习验收，不做医生或阶段 4 的故障注入器。
- 版本依据：Go 模块使用 `go list -m -json ...@latest`，监控镜像使用对应官方 GitHub 仓库的稳定发布。全部固定准确版本；当前镜像已实际拉取成功。

| 组件 | 固定版本 |
|---|---|
| OTel SDK、OTLP gRPC exporter | v1.47.0 |
| otelgin、otelgrpc | v0.72.0 |
| XSAM/otelsql | v0.44.0 |
| redisotel | v9.23.0，与 go-redis 一致 |
| prometheus/client_golang | v1.25.0，沿用阶段 2 |
| Prometheus | v3.15.1 |
| Alertmanager | v0.34.1 |
| Grafana | 13.2.4 |
| Jaeger | 2.20.0，用户批准的唯一版本例外，见下文 |
| Loki | 3.7.9 |
| Alloy | v1.21.2 |
| mysqld-exporter | v0.21.0 |
| redis_exporter | v1.95.0 |
| kafka-exporter | v1.11.0 |

- 插件依赖带入的 mimetype v1.4.15 已被上游撤回；安装时看到了撤回提示，改用未撤回的 v1.4.16。其他间接版本由 go.mod/go.sum 固定。
- 每个进程独立创建注册表和 TracerProvider，不修改全局 OTel 状态。通过构造参数或启动前注册把依赖交给适配器，单元测试不需要连接监控后端。
- 所有 Prometheus 采集器由统一 registerer 添加 service。HTTP 标签使用模板路由；未知路径与未知方法分别合并，订单号、幂等键和 trace ID 不进入指标标签。
- Kafka span 手动传播 W3C traceparent/tracestate；SQL 用插件记录模板而不记录绑定参数，Redis 关闭原始命令记录。日志写出时读取当前 span，避免子操作还打印入口 span ID。
- 沿用用户对 `needs_review` 的授权，补齐第 5.8.5 节遗漏的该标签值。它只计进入待核对的次数，重复读取不再增加。缓存读取错误单列 error，不能伪装为正常未命中。
- 消费仓储多返回一个 duplicate 标识，消费层据此区分 processed 和 duplicate；直到位点提交成功才记录成功，任何中途失败记 failed。原来的事务顺序和去重行为保持不变。
- 订单提交后的 Kafka 发送按用户要求改用 WithoutCancel 加独立超时，保留上下文值。它只能缩小客户端取消造成的漏发窗口，不能替代 Outbox；进程崩溃或 Kafka 故障仍可能缺消息。
- Jaeger 使用内存后端，最多五千条 trace；默认根请求采样十分之一，已有父节点则跟随父节点。目的是控制本机资源。放弃全量长时间追踪和引入额外存储；配置可以改为全采样做短时实验。
- 只读 metrics 数据库账号限制三个连接，只有 PROCESS、REPLICATION CLIENT 和指定库 SELECT。已有业务卷用同一个初始化脚本补建账号，不删除数据卷。Grafana 密码、监控账号密码和推送令牌从环境读取。
- Alloy 需要 Docker socket 发现和读取日志。只挂给采集器，并明确 `:ro` 不是 Docker API 的权限隔离；医生不会获得该挂载。日志采集过滤到 opsdoctor 项目的五个 Go 服务，不顺带收集其他项目。

配置参考：[Jaeger v2 配置](https://www.jaegertracing.io/docs/2.22/deployment/configuration/)、[Alloy Docker 日志源](https://grafana.com/docs/alloy/latest/reference/components/loki/loki.source.docker/)、[MySQL exporter](https://github.com/prometheus/mysqld_exporter)。按本阶段固定版本实测，不把最新版文档中的示例当成已通过的配置。

<a id="jaeger-compatibility"></a>

## 2026-10-23：Jaeger 2.20.0 的单项版本例外

**决定与授权。** 用户明确批准本阶段固定 `jaegertracing/jaeger:2.20.0`，并要求保留对照证据、解除条件和阶段 5 适配器的升级约束。这个例外只针对 Jaeger；其他组件继续按计划书第 2.6 节在阶段开始时选最新稳定版。没有引入其他追踪产品，也没有把所有依赖降到旧版本。

**怎样排除本项目配置错误。** Grafana 始终为 13.2.4，容器没有重建，Jaeger 数据源 URL 始终为 `http://jaeger:16686`。两次测试使用同一个 `deploy/grafana/provisioning/datasources/sources.yml`，SHA-256 均为 `110e07eb43712c494bd9e60b30ec2a2a9bf692082b8039e5ba7e5b7d85f212e2`。Jaeger 服务端配置也未改，只更换镜像标签。对照发生在本机 08:44 和 08:46（America/Los_Angeles）。

| 实际请求 | Jaeger 2.22.0 | Jaeger 2.20.0 |
|---|---|---|
| 直接访问 Jaeger：`GET http://127.0.0.1:16686/api/services` | HTTP 404，`404 page not found` | HTTP 200，返回 data 数组 |
| 经 Grafana 代理：`GET http://127.0.0.1:3000/api/datasources/proxy/uid/jaeger/api/services` | HTTP 404，`404 page not found` | HTTP 200，返回 data 数组 |
| Grafana：`GET http://127.0.0.1:3000/api/datasources/uid/jaeger/health` | HTTP 400，status=ERROR，message=`request failed: 404 Not Found` | HTTP 200，status=OK，message=`Data source is working` |

返回 404 的 Jaeger 路径明确是 **`GET /api/services`**；Grafana 的 health 路径返回的是 400，它在消息中报告下游的 404，不能把两者混写。2.20.0 刚重启时内存后端为空，所以最初 data 是空数组；新请求进入后列表有内容。换版本没有改变 Grafana 的配置，故障现象随 Jaeger 版本改变，与上游已确认的接口移除相符。

**上游依据。** [Grafana Jaeger 数据源问题 #87](https://github.com/grafana/grafana-jaeger-datasource/issues/87) 说明服务/操作列表、搜索仍依赖旧路径；[Jaeger v2.21.0 发布说明](https://github.com/jaegertracing/jaeger/releases/tag/v2.21.0) 和 [移除旧 HTTP 接口的变更 #9260](https://github.com/jaegertracing/jaeger/pull/9260) 记录了这个破坏性变更。2026-10-23 查询时 #87 仍为 open。没有只开一个实验开关来绕过健康检查：上游说明该开关仍留有搜索与属性解析问题。

**2.20.0 的实际查询验收。** 新订单 trace `2cd8e6e7f7ab413eb88850d3d0c18ede` 包含三个业务服务和 29 个 span。经同一个 Grafana 代理查询 `/api/services`、`/api/services/order-service/operations`、`/api/traces?service=order-service&limit=20&lookback=1h`、`/api/traces/{trace_id}` 全部返回 200 且非空。再调用 Grafana `/api/ds/query` 查询该 ID，得到一个有效 trace 数据帧，关联日志也可通过 Loki 数据源查询。CI 加入列表和搜索冒烟，避免以后只测按 ID 查询而漏掉这类兼容问题。

**解除条件。** 当 Grafana 或 Jaeger 修复该兼容问题后，升级到届时最新稳定版。先复测数据源健康、服务与操作列表、带条件搜索、trace 详情及日志跳转；覆盖真实 span 的字符串、数值、布尔属性和父子关系。阶段 5 适配器已经存在时，也必须同时通过下面的回归。全部通过后更新 Compose、计划书、这条决定及实测记录，才解除例外；不能只把镜像标签改回最新版就认定完成。

**阶段 5 的约束。** 医生会直接调用 Jaeger 查询接口，其适配器必须以本项目固定的 **2.20.0 实测** 为准，按这个版本的请求参数和返回结构编写测试。本阶段只记录约束，没有提前实现医生。以后升级 Jaeger 时，同步回归适配器的搜索、详情、属性解析、父子关系、空结果与错误处理，不得只验证 Grafana。若迁移到新 API，要同时更新适配器和测试数据，不能让医生继续依赖已经移除的路径。

**代价与限制。** 固定旧版本期间拿不到之后版本的全部修复，所以这是带解除条件的临时例外。Jaeger 使用内存存储，重建会清空旧 trace；切换期间库存与消费者的一批 span 曾因五秒导出超时未送达，新请求在连接恢复后才得到完整 trace。此失败和一次下单待核对均保留在 notes，不把它们改写成全程无故障。


<a id="grpc-dns-txt"></a>

## 2026-10-23：跳过 gRPC 的 DNS 服务配置查询

用户验收指出，阶段 3 的库存超时和 span 丢失与 `_grpc_config.<主机名>` 的 TXT 查询有关，明确要求库存客户端和 OTLP 导出器加入 `grpc.WithDisableServiceConfig()`。本项目没有通过 DNS 分发 gRPC 服务配置，因此在代码中统一禁用这项读取，继续通过 DNS 解析服务 IP。手动库存命令也使用同样的选项。OTLP 使用 `otlptracegrpc.WithDialOption` 传入，不依赖环境变量，也不改成固定容器 IP。

新增 `INVENTORY_GRPC_IDLE_TIMEOUT`，默认 30m，0s 关闭空闲回收，范围为 0 至 24h。它只控制没有 RPC 时何时回收连接，与 800ms 的库存调用预算不同。实验临时使用 10s，生产默认仍保留 30m；启动时的 connectInventory 也保留。没有用永久关闭空闲回收来绕过问题。

源码依据为仓库固定的 grpc-go v1.84.0：`internal/resolver/dns/dns_resolver.go` 的 Build 把 DisableServiceConfig 反向赋给 enableServiceConfig；lookup 先 lookupHost，再按该标志 lookupTXT，然后才交出地址。`stream.go` 在 waitForResolvedAddrs 后才进入创建 RPC 尝试和 TagRPC 的步骤。因此，解析等待已经耗尽预算的请求，可以有 HTTP span 和 Redis span，却没有 gRPC span。OTLP 导出器内部同样使用 grpc.NewClient，存在同一种等待。

源码链接：[DNS 解析顺序](https://github.com/grpc/grpc-go/blob/v1.84.0/internal/resolver/dns/dns_resolver.go#L339)、[解析器选项](https://github.com/grpc/grpc-go/blob/v1.84.0/internal/resolver/dns/dns_resolver.go#L142)、[RPC 等待解析](https://github.com/grpc/grpc-go/blob/v1.84.0/stream.go#L338)、[空闲回收](https://pkg.go.dev/google.golang.org/grpc@v1.84.0#WithIdleTimeout)。本机 DNS 查询、失败 trace 与前后对照数据另记于 notes 和数据台账。

用户也授权将 MySQL 上限提高到 1.5 GiB。保留原有 256 MiB buffer pool 和 100 个最大连接，只增加容器余量；758/768 MiB 的观测说明原限额偏紧，不能把所有数据库内存都当成 buffer pool。此时没有证据需要扩大缓存或调整更多数据库参数，避免同时改变查询缓存容量。调整后仍以有流量的 docker stats 实测为准，不把上限当成实际占用。


<a id="phase06-tool-errors-baseline"></a>

## 2026-10-27：阶段 6 开工先修工具纠错与实验基线

用户要求先单独提交这两项阶段 5 遗留修复。SELECT 校验保留具体原因；EXPLAIN 的 1054、1146、1064 错误返回最多 512 字节的原文，方便根据列名、表名和语法位置修正。其他 MySQL 错误只公开错误号。工具只认适配器标记过的 QueryError，普通后端错误仍隐藏。放弃直接公开所有数据库错误，因为连接和鉴权错误可能含账号信息。

loadgen 的 GET /control 返回本进程的 product_count、workers。chaos healthy 和 selftest 在等待指标前核对，默认要求 10000 商品、64 个工作者；自定义实验可用 CHAOS_BASELINE_PRODUCT_COUNT、CHAOS_BASELINE_WORKERS 改预期。参数不一致立即失败并给出重建办法。这样能发现“本机变量已改，但容器仍跑旧参数”的情况。保持 loadgen 的五十万商品默认值和 CacheLoadsHigh 规则不变。
