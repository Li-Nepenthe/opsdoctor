# 实测记录

<a id="phase-05"></a>

### Linux CI 40296137783 与阶段收尾

[CI 40296137783](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/40296137783)，提交 `5b26ed0ad22653034285de6f257cb9b6967d7f48`，2026-10-26 UTC 08:59:21 开始，结论 success。固定 ubuntu-24.04、Go 1.27.1。格式、tidy、vet、staticcheck、注释与架构、离线 race、缺陷版本构建、全新 Compose、真实权限集成、small 库存/订单/消息、网络恢复、三类观测接线、医生登录/工具/通知去重全部通过。真实集成输出 pass=28、skip=0（包含子用例）。医生冒烟用三次相同通知得到同一个 ID，created 恰好一次，任务是 shop/pending。

随后只补目录导航、CI证据、进度与报告清单，不改变生产源码。学习验收仍全部由用户自己勾；main 保持 dcbfe7a，阶段分支推送后停止。



### 文档与提交前核对

学习材料四段完整 Go 示例在临时模块实际运行通过；十二段 PowerShell 示例通过语法解析，没有再执行其中的故障实验。新文档的相对文件链接均存在。已知本机真实密码、密钥和令牌逐字扫描没有进入仓库文件；不保存扫描值。非 Markdown 文件按路径排序、换行归一化后计算 SHA-256 指纹：`aa3d2ffd712512f9020ca9ab40de078a76f6b47d39219e16c0bd3f210ce17591`。



### 医生接入、S01 与最终本地检查（2026-10-26）

运行代码为 phase-05-doctor-tools 的阶段 5 工作树，父提交 d4641f0；业务镜像 20261026.1，医生镜像 20261026.1，Jaeger 固定 2.20.0。以下两轮均为 10000 商品工作集、200 QPS、64 个工作者，未改 CacheLoadsHigh。S09 的认证版缺陷镜像 20261026.2 已实际构建成功，没有把这个构建说成新的一轮 S09 实验。

真实 MySQL 告警仓储：同一组顺序推送 3 次，再并发推送 20 次，仅一个 pending 任务；另一个系统的相同 groupKey 另建任务。恢复通知更新快照，不删除任务；无现有任务的单独恢复通知忽略。写快照失败整笔事务回滚；重复初始化不覆盖已有用户密码。记录 doctor-store-integration.txt。HTTP 测试另验证 A 的令牌推 B 返回 401 且不调用仓储，登录 JWT 与告警令牌不能互换。

S01 首次脚本没有填 CHAOS_MYSQL_DSN：healthy 通过后，drop_index 在打开数据库前失败；恢复也因同一缺项失败。没有改动索引。补好专用 DSN 后执行 restore-all 成功，保留 doctor-s01-preflight-failed.json，再开始正式轮次。

| 轮次 | UTC 注入开始 | 注入动作 | 从注入到告警 | 恢复动作 | 恢复后 healthy | 结果 |
|---|---|---|---|---|---|---|
| 1 | 2026-10-26T08:21:20.602036+00:00 | 0.111s | 50.138s | 6.225s | 130.266s | 全流程通过 |
| 2 | 2026-10-26T08:27:00.880327+00:00 | 0.049s | 40.080s | 5.031s | 130.218s | 全流程通过 |

第一轮搜到 GetStock 的慢链：整体 302.338ms，库存 RPC 301.584ms，SQL 查询 34.570ms；不能把这条冒充 Deduct。第二轮指定 operation=inventory.v1.InventoryService/Deduct，trace 9b4d9c769665469f010bdf86978c38d4：POST /orders 824.790ms，RPC 801.094ms，库存服务 800.757ms，UPDATE 的 sql.stmt.exec 796.733ms，九个 span 全部在树中。SQL 是按 sku_id 更新 available 的模板，不包含绑定值。

故障中 EXPLAIN 为 type=ALL、key=NULL、rows=498780（估计值）；SHOW INDEX 只有 PRIMARY。第一轮锁快照有 6 行等待，第二轮为 0；它是瞬时状态，不能把空快照说成没有发生等待。恢复后另查 EXPLAIN 为 type=const、key=uk_stock_sku、rows=1，索引列表恢复 PRIMARY 与 uk_stock_sku，见 doctor-final-audit.json。

两轮完整工具测量如下。原始大小指最终截断前的 Data JSON，结果大小包含完整 Result 字段。耗时来自 Result.duration_ms，0 表示不足一毫秒；不是整个 CLI 进程启动时间。工具先按用途摘要，后面还有统一 4096 字节上限。

| 工具/参数区别 | 首轮 ms | 首轮原始→结果字节 | 首轮截断 | 第二轮 ms | 第二轮原始→结果字节 | 第二轮截断 |
|---|---|---|---|---|---|---|
| list_firing_alerts | 3 | 868→1026 | False | 1 | 869→1027 | False |
| service_topology | 0 | 3452→3607 | False | 0 | 3452→3607 | False |
| query_metrics | 1 | 134→345 | False | 1 | 134→345 | False |
| search_traces | 58 | 1529→1756 | False | 2 | 1550→1776 | False |
| get_trace | 3 | 840→1030 | False | 2 | 1090→1281 | False |
| mysql_explain | 3 | 241→436 | False | 1 | 241→436 | False |
| mysql_show_indexes | 4 | 162→359 | False | 3 | 162→359 | False |
| mysql_table_stats | 2 | 134→357 | False | 1 | 134→357 | False |
| mysql_top_queries | 256 | 415→635 | False | 2 | 468→686 | False |
| mysql_lock_waits | 821 | 473→691 | False | 1 | 150→366 | False |
| mysql_status | 0 | 271→453 | False | 0 | 271→453 | False |
| search_logs / 10条 | 310 | 423→603 | False | 121 | 450→630 | False |
| search_logs / 100条 | 12 | 40199→3819 | True | 29 | 40274→3818 | True |
| redis_info / stats | 1 | 2490→2648 | False | 0 | 2490→2648 | False |
| redis_info / slowlog | 0 | 129→286 | False | 7 | 129→286 | False |
| goroutine_summary | 5 | 912→1107 | False | 3 | 4137→4079 | True |
| kafka_consumer_lag | 0 | 715→931 | False | 0 | 715→931 | False |
| recent_changes | 2 | 1647→1904 | False | 5 | 2631→2888 | False |

首轮 Alertmanager 实际新增 3 个任务：InventoryLatencyHigh、DatabasePoolWaiting、RowLockWaiting，全部 target=shop、status=pending，每个保存一条告警快照。第二轮复用已有活动分组，未新建重复任务。first trace 与原始任务明细只保存在本机临时记录，正文不包含令牌。

只读连接通过 root 的独立观察连接查询 performance_schema.threads 中 PROCESSLIST_USER=diag_ro 的条数，查询后等待 250ms 再采样，实际间隔还包含 docker exec 耗时。第一轮 153 个样本、第二轮 122 个样本，观测最大均为 1。另从同一 HTTP 工具入口发送 12 路/120 次 mysql_status、8 路/24 次 mysql_top_queries，全部 ok。短请求之间可能没有被采样捕捉到，不能把观测最大 1 说成连续监测的精确峰值。账号第四条连接被拒绝、客户端池最多三条和满池十秒超时另由真实集成测试证明。

19 个运行容器：首轮 S01 快照 3411.730 MiB，doctor 13.73/256 MiB、doctor-mysql 200.6/512 MiB；第二轮 3587.000 MiB，doctor 18.64/256 MiB、doctor-mysql 200.8/512 MiB。均低于 4608 MiB 暂停线。这是两次 docker stats --no-stream 快照，不是长时间峰值。全部逐容器数在 doctor-s01.json、doctor-s01-deduct.json 的 stats.rows。

最终只读部署核对：doctor 仅有 /etc/doctor/targets 的只读 bind mount；容器无 /src、/eval/scenarios、/.opsdoctor-chaos.json、/var/run/docker.sock。archcheck 还检查 doctor 不得 import shop/chaos 或启动 os/exec，不能读取静态可识别的场景路径和状态文件，不能给容器增加挂载、env_file 或 Docker/chaos 环境变量。源码静态分析不能识别任意动态路径，因此不能代替容器挂载限制。

全量检查第一轮在 staticcheck 停下：后端名称开头的错误字符串格式及测试中的数字 302。修正为中文错误上下文与 http.StatusFound 后，从 gofmt 开始重跑。首轮日志另存 first-attempt-final-*，环境仍正常恢复。

最终本地 gofmt、go vet、staticcheck、commentcheck（243 个非生成 Go 文件）、archcheck 全通过。go test -race ./... -count=1 -timeout=180s 无外部依赖轮：136 个顶层测试通过，534 个通过事件含子用例，22 个明确依赖外部环境的集成测试跳过，42 个有测试包通过，墙钟 16.616s。随后注入全部 11 个 TEST 变量，十个包的真实 Integration -race 轮：22 个顶层测试、28 个含子用例通过，0 skip，0 fail，墙钟 15.253s。两份 promtool 时间线测试通过。测试前暂停负载、等消费追平、停止医生和通知；完成后启动医生、恢复 200 QPS，healthy 120.261s 通过，再恢复 Alertmanager。

记录：final-checks.json、final-unit.txt、final-integration.txt、final-promtool-*.txt、doctor-final-audit.json。流水线与文档示例验证在后续收尾记录补充，不用本地通过替代 CI。



### Redis 认证改动与失败保留（开发日期 2026-10-26）

用户批准了 default 账号补密码，并要求保留数据重建、验证匿名拒绝和只读权限、完整回归 S04/S05。密码只补进本机忽略的 .env。业务正常镜像改为 20261026.1，S09 的另一构建改为 20261026.2，避免旧镜像不认识密码配置。

实际执行 compose down（8.793s）和 up --wait（23.226s），没有 -v。Redis 键数从 788310 变为 788795；启动后流量已恢复，不能据此逐键证明完整性，但明确没有清卷或清库。全部容器就绪，redis_up=1。

权限测试首次被自动审批拒绝，原因是它含 FLUSHALL，不能在 ACL 错误时承担删数据副作用。随后独立读取并过滤 ACL（不输出密码哈希），先执行七条 ACL DRYRUN，全部明确拒绝。实际集成测试的第一次运行因把 DRYRUN 普通拒绝字符串误当成错误而停止，没有执行负向命令；修正解析后通过（包 1.575s，测试 0.07s）：匿名 NOAUTH；INFO、SLOWLOG 成功；GET、HGETALL、KEYS、SET、FLUSHALL、CONFIG、SLOWLOG RESET 均 NOPERM。前后失败与成功日志都保留。

认证回归第一次在任何故障注入前失败：healthy 从 UTC 07:00:11.321 到 07:03:11.626，180.305s 结束，最后条件为 CacheLoadsHigh。compose 重建后 loadgen 使用默认 500000 商品；随后一次控制接口快照为 575.657s、115127 请求、0 错误、0 dropped、199.992 QPS、P99 90ms，告警值 43.8727%。用户选择仅按旧 10000 商品基线复验，规则及默认值未改。这次失败不能删掉或记成通过。

原始文件均在临时目录 opsdoctor-phase05：redis-auth-restart.json、redis-acl-preflight.json、redis-auth-permissions.txt（失败）、redis-auth-permissions-retry.txt（通过）、redis-auth-s04-s05.json（50 万商品，尚未注入即失败）、redis-auth-500k-baseline.json、redis-auth-10k-s04-s05.json（另行复验）。

### 认证后的 S04/S05 完整流程

用户确认的复验参数为 **10000 商品、200 QPS、64 个工作者**。三个值已写入本机 .env；重新创建 loadgen 后才开始复验。正常构建 20261026.1，规则保持 main 的 CacheLoadsHigh 表达式。

| 流程 | 注入动作 | 从注入到告警 | 恢复动作 | 恢复后 healthy | 结果 |
|---|---|---|---|---|---|
| 初始健康 | — | — | — | 60.122s | 通过，redis_up=1 |
| S04 | 52.724s | 82.466s | 5.961s | 85.170s | inject / verify / restore / healthy 全通过 |
| S05 | 24.605s | 94.397s | 5.901s | 90.175s | inject / verify / restore / healthy 全通过 |

两次 healthy 最后等待的都是 CacheLoadsHigh 消退，然后连续健康窗口通过。S04/S05 的症状、持续时间和三分钟上限没有放宽。另从 Prometheus 取回默认 50 万商品失败窗口（UTC 07:00:11.321–07:03:11.626）的 37 个样本：回源比例 44.500%–48.447%，均值 46.291%；此前 43.8727% 是另一次后续快照，不能混写为整个窗口均值。

## 2026-10-25：阶段 5 开工与验收遗留修复

用户明确授权阶段 5，完成后等 Claude 验收和用户学习。先读 AGENTS.md、进度及当前完整的 2062 行计划书。工作区初始干净，fetch 后 main 与 origin/main 都为 `dcbfe7a6a0165a5f6e40bcd3240604b7228ca48b`，从这里创建 `phase-05-doctor-tools`。没有合并 main，也没有勾学习验收。

先做四项遗留修复，并单独提交。本机原有 18 个容器已经停止约 17 小时，执行 `compose start --wait --wait-timeout 240` 后全部健康。冷启动原始样本放在本机临时目录 `opsdoctor-phase05/cold-original.jsonl`；后续每轮使用不同文件名，失败数据也不覆盖。以下实验时间用 UTC，开发日期沿用用户本地的 2026-10-25。

`TestRetryPolicy` 的五个表格用例放进标准库 `testing/synctest.Test`。它提供虚拟时钟，只有测试里的 goroutine 全部阻塞时才推进时间。本测试的假等待立即返回，因此 1ms 预算不会被真实调度耗尽。没有修改生产重试代码，也没有修改预期次数和等待序列。`go test -race ./internal/platform/resilience -run '^TestRetryPolicy$' -count=1000 -timeout=180s` 通过，包输出 2.266s；1000 次顶层测试、5000 个表格子用例。依据：[Go 官方 synctest 文档](https://pkg.go.dev/testing/synctest)。

本机 `.env` 确实缺 `TEST_ORDER_MYSQL_DSN`、`TEST_SALES_MYSQL_DSN`、`TEST_REDIS_ADDR`。补前两个时沿用既有测试账号，只把库名换成两个专用库；Redis 指向本机映射端口。没有输出密码，`git check-ignore .env` 确认仍被忽略。README 列全七个变量，要求预检、保存运行记录、确认 `pass>0 / skip=0 / exit=0`；CI 也检查跳过事件。说明了共享 MySQL 实例会带来库存延迟、下单延迟、行锁三类告警，以及暂停通知、恢复健康、保留已有 pending 任务的操作顺序。

初步检查：gofmt、vet、commentcheck（179 个非生成 Go 文件）、archcheck 通过。第一次直接调用 staticcheck 时 PATH 中找不到它；通过 `go env GOBIN` 找到已安装的 `staticcheck.exe` 后通过，没有重新安装。12 条告警配置、两份 promtool 测试通过。新测试包含冷启动稳定、持续泄漏、旧进程增长、错误实例匹配、重启五种时间线；人工数据不能作为实际耗时引用。

### 冷启动与 S09 的真实对照

目标流量 200 QPS，活跃商品范围 10000，64 个工作者。未调整 OrderLatencyHigh。

| 轮次 | UTC 时间与窗口 | 结果 |
|---|---|---|
| 原规则，已停止约 17h 的整套环境启动 | 2026-10-26 05:31:08.206–05:36:05.647；样本窗口 297.441s，60 次 | 订单 goroutine 最大 116，增长率最大 1.981/s；超过 1 的最后一个样本在启动 64.976s。GoroutinesGrowing 进入 pending、没有 firing；OrderLatencyHigh firing，OrderListLatencyHigh pending |
| 90s 启动保护，整套 stop/start | 停启 05:37:01.767–05:37:33.532，31.765s；采样 05:36:49.533–05:44:45.807，476.274s、95 次 | 停止期间 1 次 Prometheus 不可用；其余 94 次没有 pending/firing。订单增长率峰值 0.331/s、数量峰值 110；本轮本身未超过原门槛，另外用合成测试确认保护逻辑。独立 healthy 60.126s 通过 |
| S09 完整流程 | 05:45:05.123 开始命令，05:50:38.156 完成恢复健康 | inject 6.228s；verify 报从注入记录起 161.078s 出现 GoroutinesGrowing；restore 6.271s；healthy 165.382s。四步都成功 |

S09 命令输出原样保留：

```text
S09 动作已执行；请继续 verify，不能据此认定症状已出现
S09 symptom=true alerts=[GoroutinesGrowing] elapsed=2m41.078s
healthy elapsed=2m45.382s last=健康条件 6 尚未满足
```

恢复健康接近三分钟，最后曾未满足的条件是消费积压小于 10；不能把恢复镜像的 6.271s 当成全部业务恢复耗时。当前源码尚未重建业务镜像，S09 对照为已有正常 20261024.3 与缺陷 20261024.4；本批生产变化只有 Prometheus 规则，业务重试只改测试。

原始文件：`cold-original.jsonl`、`cold-guarded.jsonl`、`cold-summary.json`、`restart-guarded.json`、`s09-guarded.json`、`s09-guarded.jsonl`，都在本机临时目录 `opsdoctor-phase05`。原规则未复现 firing 的事实和用户此前的误报记录分开记。新进程保护期不是普适结论，今后更慢机器需要重测。

新增医生前，18 个容器有目标 200 QPS 时的 docker stats 合计为 2212.437 MiB，逐容器原始值在 `stats-existing.jsonl`。这是刚重启后的快照，不是峰值，也不能与旧长时间运行快照直接计算优化比例。

全量无外部依赖 race：109 个顶层测试通过，16 个依赖 MySQL/Redis 的集成测试跳过，记录 `leftovers-unit.jsonl`。随后补齐七项变量、暂停负载与 Alertmanager，真实集成轮 16 个测试全部通过、0 跳过、退出码 0，测试墙钟 8.051s，记录 `leftovers-integration.jsonl`。两轮合计覆盖 125 个顶层测试，但不把跳过轮写成“零跳过”。 测试结束恢复 200 QPS 后，healthy 等待 135.291s 通过；期间观察到 RowLockWaiting，随后清除。通过后才重新启动 Alertmanager，通知恢复。

<a id="phase-04"></a>

## 2026-10-24：阶段 4 开工

用户明确授权进入阶段 4，完成后等 Claude 验收和用户学习。完整读完当前 2045 行计划书，包括第 3.6、4.3、5.4 节。Git 实际显示 d55b9eb 到 0c96b0a 共五个提交（用户描述为六个），全部是文档和注释。工作区干净，获取远端后，将 phase-03-observability 快进合入 main，推送 main 与阶段 3 分支，再从 0c96b0a 创建 phase-04-chaos。学习验收保持未勾选。

开始时 Docker Desktop 引擎未运行，通知用户后由用户启动。首先增加消息未确认告警的错误分类和等待耗时，保留原发送参数用于复现。读取 kafka-go v0.4.51 writer.go，确认调用者的 ctx 只决定是否继续等待批次；批次自身用另一份 context 执行，即使调用者超时，批次仍可能完成。因此“未确认”和“后来消费成功”并不矛盾。旧告警未保留底层错误，不能仅凭这段源码断言那 53 次的最初触发原因。

按用户授权新增显式 stock restock 命令，通过现有 gRPC Release 补库存，失败不自动重试、不直接改表。Jaeger 容量先设为 30000 条，200 QPS、0.1 采样的容量计算约为 25 分钟；实际保留时间和内存待持续流量验证。

### 消息发送预算调整前的五分钟基线

源码 515ca525，APP_VERSION=20261024.1，确认等待仍为 2 秒。业务数据保留 50 万商品和 200 多万订单。实验前暂停流量，用正式 stock restock 为 SKU 1–10000 各补 100000 件，10000 次均确认成功；本轮 loadgen 商品范围为 10000，hot_sku_id=0，目标 200 QPS，64 个 worker。原始结果保存在本机临时目录 opsdoctor-phase04/kafka-before.json 及同名前缀日志。

从开始流量到暂停完成为 302.194 秒，共 60151 请求，按实际窗口约 199.05 QPS；dropped=289，HTTP 错误 5，P99=237ms。消息成功确认 12180 次，消息未确认告警 0。不能因为基线没复现就声称旧 53 次不存在。第一次采样前出现两次缓存写入失败、一次结果保存失败进入待核对（订单 2085510、商品 34），待核对状态当时也未能写回 Redis；这些是本轮真实失败，不能写成全程健康。后续确认预算调整不会被当成这些 Redis 失败的修复。

Jaeger 在本轮最后一次快照为 88.77 MiB / 1 GiB；开头带指定 trace_id 的请求在五分钟后仍能查到 12 个 span。这只能证明五分钟，十五分钟目标仍待长实验验证。

### 长实验第一次中断

预算调整后的源码为 4da5da10，订单 APP_VERSION=20261024.2。计划运行 1200 秒，但只收集到 318.707 秒时的十个采样，随后 Docker Linux 引擎不可连接，全部 17 个容器显示 Exited (255)。无法据此推断引擎退出原因。用户重新启动 Docker Desktop 后，先恢复原容器，再重新做完整实验。

中断前 loadgen 累计计数从 60153 增到 123896；错误仍为 5，dropped 仍为 289，本轮采样区间没有新增错误或丢弃。最后 Jaeger 为 153.5 MiB，起始标记仍可查到七个 span。原文件 kafka-after.json 保留，缺少 ended_utc 和最终 elapsed，不能作为十五分钟验收；新轮次使用不同文件名，不能覆盖它。

### 阶段 4 首版代码检查

新增 scenario/action/verify/app 四个包、十一份场景文件和命令入口。先写数据结构与白名单，再写外部适配器、保存恢复记录、取消后的恢复流程与监控判断。用户批准的单次 GetStock 时限已接入；默认零保持原 deadline，配置上限十次重试，Deduct 在全部允许配置下仍只调用一次。新增测试实际通过。

首轮全量检查：gofmt、go vet、staticcheck、commentcheck、archcheck 通过；commentcheck 为 174 个非生成 Go 文件。无外部依赖全量 race 的 JSON 输出为 338 个通过测试事件（包含子用例）、29 个通过测试包，16 个集成测试跳过，另 11 个包没有测试函数。原始输出 unit-initial.jsonl；不把跳过的集成测试算通过。另执行 `go build -tags leakbug`，另一构建版本编译通过。

Prometheus `promtool check rules` 检查十二条规则通过，但此时尚未重载，不代表真实场景触发通过。新 YAML 依赖沿用已有的 goccy/go-yaml v1.19.2，由间接转为直接；实际 `go list -m -json github.com/goccy/go-yaml@latest` 返回同版本。学习材料四个完整 Go 示例都在临时目录实际运行并 gofmt，输出已原样写入材料。

### 二十分钟复验完成

Docker 重启后，仍使用源码 4da5da10 和 APP_VERSION=20261024.2。2026-10-25T05:04:48Z 开始，实际流量窗口 1201.583 秒，240109 请求、0 错误、218 dropped，实际 199.827 QPS，P50 3ms、P99 152ms。商品/下单/列表分别 168145、47856、24108 次。订单日志没有“消息未确认发送”告警，消费者日志共 47856 行；最后采样时发送计数为 47855，采样到暂停之间又完成一笔，不能把两个不同时刻的计数差误判成丢消息。

初始 trace `3a11fad8e537497fb011e028965e7723` 在第 1200.001 秒仍能查到 12 个 span。Jaeger 最后占用 256 MiB / 1 GiB，十五分钟保留目标通过本轮实测。原始文件 kafka-after-rerun.json 和四份同名前缀日志保留；中断轮没有被覆盖。两秒基线也没有重现旧 53 条，因此只能说新预算通过本轮测试，不能声称证明历史每一条的触发原因。

### 代理部署、集成测试与首轮场景实测

源码 ffccf03。18 个容器全部通过 Compose 健康检查，Toxiproxy 2.13.0 的三个代理已接入。Prometheus 配置和 12 条告警规则通过 promtool；HTTP reload 接口未启用，调用返回拒绝，随后用 SIGHUP 成功加载，未开放额外管理接口。S09 的独立镜像 20261024.4 编译成功，正常版本为 20261024.3。

第一次集成测试只从本机 .env 读到了四个旧 TEST DSN，7 个通过、9 个因参数缺失跳过。随后仅在测试进程里补齐订单、销量测试库参数和 Redis 地址，再跑完整的 Integration：16 个顶层测试通过，0 跳过。没有把专用测试 DSN 指向 shop 业务库。

这一轮集成测试与最初的 healthy 检查重叠。并发扣库存测试会影响 MySQL 全局行锁等待计数，触发 RowLockWaiting，因此 healthy 在 3m0s 超时。这次失败保留，不计为业务场景故障。测试结束后锁等待速率回到约 0.086ms/s，告警消退，再启动首轮 selftest。后续整套实验期间不并行执行数据库测试。

首轮 S01 于本机 22:33:45 开始。期间只读执行 `EXPLAIN UPDATE stock SET available=available-1 WHERE sku_id=1 AND available>=1`，得到 type=index、key=PRIMARY、rows=498780、Using where：UPDATE 扫描主键索引中的大量记录，不能把它写成精确索引定位。与阶段 1 对 SELECT 的 type=ALL 结果不是同一种语句。未执行该 UPDATE。过程指标和完整结果表保存在 selftest-initial-metrics.jsonl 与 selftest-initial.txt，后续按真实结果补齐。

S02 期间只读查看列表查询的执行计划，返回 type=ALL、rows=1991940、Using where; Using filesort。查询内容为 `EXPLAIN SELECT id,user_id,sku_id,quantity,status,created_at FROM orders WHERE user_id=1 ORDER BY created_at DESC,id DESC LIMIT 20`。没有执行数据修改。首个较完整指标窗口中列表 P99 约 440ms，而商品详情约 9ms、下单约 49ms；受影响入口不同，是定位列表查询的第一条线索。

### 首轮 selftest：原样保留失败

源码 ffccf03，完整输出文件 selftest-initial.txt。首版遇到恢复失败后只打印已执行行；下面补上未执行编号，明确它们没有被测试，非测量值用破折号。后续版本会直接打印这些未执行行。

| 场景 | 事前健康 | 注入耗时 | 症状及告警（自注入起） | 恢复健康（含恢复动作） | 告警 | 结果 |
|---|---:|---:|---:|---:|---|---|
| S01 | 1m0.124s | 40ms | 50.071s | 2m5.594s | InventoryLatencyHigh | 通过 |
| S02 | 1m0.124s | 18ms | 45.14s | 2m12.247s | OrderListLatencyHigh | 通过 |
| S03 | 1m0.131s | 25.954s | 1m35.99s | 2.006s | DatabasePoolWaiting | 恢复动作 deploy：docker 命令未完成：exit status 1（原始输出未回显，避免暴露环境变量） |
| S04 | — | — | — | — | — | 未执行：S03 恢复失败后停止 |
| S05 | — | — | — | — | — | 未执行：S03 恢复失败后停止 |
| S06 | — | — | — | — | — | 未执行：S03 恢复失败后停止 |
| S07 | — | — | — | — | — | 未执行：S03 恢复失败后停止 |
| S08 | — | — | — | — | — | 未执行：S03 恢复失败后停止 |
| S09 | — | — | — | — | — | 未执行：S03 恢复失败后停止 |
| S10 | — | — | — | — | — | 未执行：S03 恢复失败后停止 |
| S13 | — | — | — | — | — | 未执行：S03 恢复失败后停止 |

S03 启动日志确认 db_max_open_conns=2、db_max_idle_conns=2，版本为 20261025.054138。它在 95.990 秒观察到连接池等待告警，但恢复镜像失败：Docker 尝试查找原 sha256 ID 时返回 No such image，甚至把它解释成待拉取的名称。镜像列表确认原 ID 已没有本地引用。这里是注入器的恢复缺陷，不能写成场景验收通过。

先暂停流量，保留原恢复记录到本机临时文件 S03-failed-original-record.json。随后从 ffccf03 导出临时源码，使用原 APP_VERSION=20261024.3 重建正常库存版本。新镜像 ID 不同，不能冒称找回了原镜像；这是按原源码、版本和公开配置做的人工恢复。修正恢复记录后执行 restore-all，再恢复原 200 QPS。后续代码在发布前给原镜像增加独立本地标签，恢复前检查内容 ID，禁止拉取或构建；增加两个针对这次失败的测试。

### 阶段 4 的 18 容器快照

第二轮 S02 恢复观察期间，下一场尚未注入，控制流量为 200 QPS。docker stats 单次快照合计 3088.301 MiB，约 3.02 GiB；配置上限合计 6464 MiB。不是峰值，也不包含整个 WSL/Windows。原始文件 stats-after-s02.jsonl。MySQL 当时约 996.9 MiB / 1536 MiB，Loki 431.4 MiB / 512 MiB，单个组件的余量仍要单独观察。

| 容器 | 实际内存 | 上限 |
|---|---:|---:|
| inventory-service | 15.52MiB  |  128MiB |
| loadgen | 18.84MiB  |  128MiB |
| order-service | 23.71MiB  |  256MiB |
| order-worker | 15.59MiB  |  128MiB |
| shop-mysql | 996.9MiB  |  1.5GiB |
| toxiproxy | 18.34MiB  |  128MiB |
| alertsink | 16.52MiB  |  64MiB |
| jaeger | 332.8MiB  |  1GiB |
| kafka-exporter | 16.96MiB  |  64MiB |
| redis-exporter | 11.77MiB  |  64MiB |
| alloy | 164.2MiB  |  256MiB |
| alertmanager | 14.86MiB  |  64MiB |
| mysqld-exporter | 9.371MiB  |  64MiB |
| redis | 84.02MiB  |  256MiB |
| kafka | 647.7MiB  |  1GiB |
| loki | 431.4MiB  |  512MiB |
| prometheus | 122.4MiB  |  512MiB |
| grafana | 147.4MiB  |  256MiB |

### 镜像恢复修复的实机回归

第二轮使用源码 46212ca。S03 在 96.728 秒确认连接池等待告警，126.090 秒恢复到连续一分钟健康。恢复后的容器引用 `opsdoctor/inventory-service:saved-b96ca0b0893b0f4338ea35b5cbc9948ad3e66ab2e030548423f0b6589744c851`，docker inspect 返回的内容 ID 与保存记录一致。真实容器回归证明先保留镜像引用、再核对内容的流程可用；完整第二轮结果另列，不把这一个通过项算作整轮通过。

### S04 首次实测暴露的场景边界

第二轮的 S04 未在三分钟内触发 CacheLoadsHigh。新订单进程的 cache_loads_total 首次完整采样为 43，之后约两分钟一直是 43；这 43 次包含部署后、热点切换前的正常请求，不能全部归因于那一次缓存删除。对应 Prometheus 查询结果保存为 S04-cache-counter.json。全过程采到的缓存回源速率最高约 18.564 次/秒，随后归零；没有用“不相关告警也响了”替它过关。

400 QPS 的热点设置同时改变商品查询和下单的 SKU。行锁等待速率最高约 19793.583ms/s，实际触发的是 RowLockWaiting、InventoryLatencyHigh、DatabasePoolWaiting，属于热行竞争的影响。按计划书要求先询问用户，没有把不相关的行锁告警算作缓存告警。

用户随后批准只针对商品查询的有限突发，并明确同键并发回源计数只能用于诊断，不能触发告警。开始补 `/burst` 控制接口：固定工作者全部接到任务后一起发出商品 GET；批次不能叠加，完成数和失败数按真实 HTTP 结果记录。对应单元与竞态测试通过，commentcheck 为 176 个非生成 Go 文件。当前整套 selftest 仍按启动时的旧场景继续运行，已有业务容器未切换这份新实现，避免在同一轮中途换实验定义。


### 第二轮完整 selftest：46212ca，十项通过、S04 未通过

开始 Unix 时间 1792907358.15219，结束 1792910313.022603，整轮约 2954.870 秒。业务容器仍使用这轮开始时的实现；后半段新增的商品突发代码未部署到这些容器。十一项全部执行，S04 的三分钟观察失败保留。原始输出 selftest-after-image-fix.txt，逐十秒指标为同名前缀的 metrics.jsonl。

| 场景 | 事前健康 | 注入耗时 | 症状及告警（自注入起） | 恢复健康（含恢复动作） | 告警 | 结果 |
|---|---:|---:|---:|---:|---|---|
| S01 | 1m0.144s | 17ms | 50.095s | 2m7.435s | InventoryLatencyHigh | 通过 |
| S02 | 1m0.153s | 19ms | 45.115s | 2m21.875s | OrderListLatencyHigh | 通过 |
| S03 | 1m35.331s | 26.689s | 1m36.728s | 2m6.09s | DatabasePoolWaiting | 通过 |
| S04 | 1m0.126s | 33.03s | 3m0.001s | 2m6.872s |  | 症状或相关告警未在时限内出现：context deadline exceeded |
| S05 | 1m0.124s | 29.057s | 1m24.081s | 1m36.094s | CacheLoadsHigh | 通过 |
| S06 | 1m0.116s | 454ms | 55.48s | 1m20.986s | ServiceUnavailable | 通过 |
| S07 | 1m0.111s | 8ms | 1m30.056s | 1m55.188s | HTTPErrorRateHigh | 通过 |
| S08 | 1m25.178s | 6ms | 45.031s | 2m5.24s | InventoryLatencyHigh,RowLockWaiting | 通过 |
| S09 | 2m5.241s | 6.521s | 1m26.569s | 1m51.733s | GoroutinesGrowing | 通过 |
| S10 | 1m0.108s | 4ms | 45.031s | 2m5.207s | InventoryLatencyHigh | 通过 |
| S13 | 1m0.111s | 29.63s | 1m34.666s | 1m56.456s | InventoryRequestsAmplified | 通过 |

### S04 初步对照与批次大小调整

使用同一份订单镜像 7528a57e，只切换 CACHE_SINGLEFLIGHT；普通流量暂停。Windows 临时 loadgen 使用新 `/burst`，只发商品 GET。先测一件商品、64 次查询：开启时回源 1 次，关闭时 2 次，客户端 P99 为 25ms / 38ms，均无错误。这说明只有一次很短的失效窗口时，便宜的主键查询未必带来明显压力。

随后用 100 个热门商品逐批比较，每批 64 次、间隔 200ms，总共 6400 次请求。开启组用时 20.043s，回源 100 次，窗口内商品 SQL 平均 4.989 次/秒，P99 29ms；关闭组用时 20.047s，回源 4952 次，平均 247.025 次/秒，P99 47ms。两组错误和 dropped 均为 0，行锁等待速率为 0。关闭组出现 CacheLoadsHigh；开启组没有缓存告警，但随后出现库存 DatabasePoolWaiting。历史 ALERTS 查询确认这条通用告警确实 firing，不能只把采样时的 pending 当成没响。

因此保留这组数据，但不把它作为最终的干净对照。继续把每批并发降到 16，把同样 6400 次查询分为 400 批，每批间隔 50ms。正常组实测 20.322s、回源 400 次、平均商品 SQL 19.683 次/秒、P99 16ms、0 错误、0 dropped、行锁等待为 0，采样时没有告警。原始文件均以 S04-prototype- 开头。分钟级 Prometheus 速率还可能包含前后预热，正式对照会单独保留观察窗口；精确回源差值与客户端 P99 已来自本次真实请求。

16 并发的关闭组随后完成：20.278s 内 6400 请求，回源 6316 次，商品 SQL 平均 311.476 次/秒，P99 20ms，0 错误、0 dropped。只出现 CacheLoadsHigh firing，行锁速率为 0。两组分钟窗口 MySQL SELECT 速率为 136.156 / 289.879 次每秒，这包含库存查询与监控查询，不等于纯商品 SQL。正常组历史查询 `max_over_time(ALERTS{alertstate="firing"}[85s] @ 1792911030)` 返回空，覆盖起始到结束后一段观察；原始结果保存为 S04-control-16-alarms.json。

保留每秒 50 次的缓存回源告警门槛，不对同键并发诊断计数报警。候选场景使用 400 个热点、每批 16 次。最大批数仍有限（500），动作平均速率限制为 320 次/秒，低于默认 HTTP 限流。普通流量先暂停，预热与删除只针对这一组商品，实验后恢复。未修改通用连接池告警来迁就 64 并发的对照，未向业务 SQL 添加延迟。正式 Compose 版本仍需重新构建后复验。

### 2026-10-25：S04 容器对照与完整检查

源码 a54dc18，正常 APP_VERSION=20261025.1；同一镜像只切换 CACHE_SINGLEFLIGHT。由 Compose 内的 loadgen 发请求，固定工作者 64 个，本实验每批只使用 16 个；400 个热点各经历一次缓存删除，批次间隔至少 50ms。普通流量暂停。先通过真实查询预热，再删除缓存，比较完全相同的查询批次。

| 指标 | 开启 singleflight | 关闭 singleflight |
|---|---:|---:|
| 商品请求 / 错误 | 6400 / 0 | 6400 / 0 |
| 实际窗口 | 20.655s | 21.301s |
| 商品数据库回源 | 400 | 6295 |
| 同键重叠查库（仅诊断） | 0 | 5880 |
| 商品 SQL 平均速率 | 19.365/s | 295.521/s |
| MySQL 全库 SELECT 数 | 6818 | 12715 |
| MySQL 全库 SELECT 平均速率 | 330.084/s | 596.910/s |
| 服务端 P99，桶内插值估计 | 47.794ms | 85.799ms |
| P99 所在桶的上界 | 50ms | 100ms |
| 全局行锁等待增量 | 0ms | 0ms |
| 后续八十秒 firing 告警 | 无 | 仅 CacheLoadsHigh |

HTTP P99 从该实验前后服务端直方图桶的差值计算，包含且只包含本批 6400 次商品查询，使用桶内线性插值，不能当成精确到小数点后三位的单请求耗时。全库 SELECT 包含查库存、查商品与监控，不等于商品 SQL。数据库计数从 SHOW GLOBAL STATUS 读取，没有修改表。原始 JSON、八十秒告警观察与每桶差值分别保存在 S04-compose-control-a54dc18.json、S04-compose-fault-a54dc18.json。两组都没有行锁告警，诊断计数未出现在任何告警规则中。

格式、vet、staticcheck、commentcheck（179 份手写 Go）、archcheck 通过。第一次完整 race 脚本把两个专用测试库名写成 shop_order_test / shop_sales_test，五项测试在连接前被保护拒绝，没有连接或清理任何库。修正为 README 中的 opsdoctor_order_test / opsdoctor_sales_test 后，完整 `go test -race ./... -count=1 -timeout=180s` 通过：125 个顶层测试，其中 16 个 Integration，0 失败、0 跳过。原错误输出保留为 all-race-a54dc18.jsonl，正确结果为 all-race-a54dc18-corrected.jsonl。

两轮长自测开始前，通过正式接口查询 SKU 1 为 84111，再执行一次 restock 加 100000，确认后查询为 184111。只在实验前执行一次，没有直接改表，也不会在 selftest 中自动补库。

第一次准备检查在三分钟后失败，尚未开始 selftest。原因是重建 loadgen 时遗漏了之前实验使用的 LOADGEN_PRODUCT_COUNT=10000，回到了默认 500000；200 QPS 下回源速率约 85.408/s，持续触发 CacheLoadsHigh。失败保留为 healthy-before-final.txt。恢复原 10000 活跃商品、64 个工作者基线后重新准备。业务数据库仍有五十万商品，没有缩小表；当前静态阈值按这个活跃范围校准，扩大范围或流量时需要重新测量，不能宣称适用于所有负载。

### 扩容前候选轮：S04 症状通过，但恢复被 Redis 满载阻挡

源码 a54dc18，开始 Unix 时间 1792912731.4396634，结束 1792913770.2310944，整轮 1038.791 秒。S04 在 63.397 秒出现 CacheLoadsHigh，恢复发布动作已结束，但 HTTP 错误比例在三分钟内未连续达标；后续七项没有执行。这轮不能计为成功，也没有继续启动第二遍。

恢复后只读排查：Redis used_memory=134248120 字节，maxmemory=134217728 字节，策略 noeviction。暂停流量时统计到 299587 个 idempotency:* 键；INFO errorstats 的累计 OOM 为 14997（不是本轮独有的错误数）。一分钟 HTTP 窗口内，下单 503 约 36.733 次/秒、201 约 6.919 次/秒。日志同时有缓存写回失败、幂等结果待核对。这些证据支持“长实验的未过期记录填满写入预算”，不能归因于商品查询产生行锁。

暂停流量后，把 Redis 写入上限改为 512 MiB、容器限额改为 768 MiB，保持 noeviction、AOF 和 24 小时幂等期限。重建沿用原持久卷，重启前后 idempotency:* 都为 299587 个；商品缓存会自然到期，总 key 数不是同一时刻的恒定值。没有 FLUSH、删除幂等记录或修改业务表。新的连续两轮另起文件保存。

原始输出 selftest-final-1.txt、逐十秒采样 selftest-final-1-metrics.jsonl、redis-full-memory.txt。

| 场景 | 事前健康 | 注入耗时 | 症状及告警（自注入起） | 恢复健康（含恢复动作） | 告警 | 结果 |
|---|---:|---:|---:|---:|---|---|
| S01 | 1m0.12s | 64ms | 55.111s | 2m3.45s | InventoryLatencyHigh | 通过 |
| S02 | 1m0.122s | 27ms | 55.105s | 2m11.82s | OrderListLatencyHigh | 通过 |
| S03 | 1m0.122s | 27.953s | 1m12.978s | 1m56.256s | DatabasePoolWaiting | 通过 |
| S04 | 1m0.119s | 58.393s | 1m3.397s | 3m0s | CacheLoadsHigh | 未持续达到健康基线：健康条件 5 尚未满足：context deadline exceeded |
| S05 | — | — | — | — | — | 未执行：前序基线或恢复未通过，或实验被取消 |
| S06 | — | — | — | — | — | 未执行：前序基线或恢复未通过，或实验被取消 |
| S07 | — | — | — | — | — | 未执行：前序基线或恢复未通过，或实验被取消 |
| S08 | — | — | — | — | — | 未执行：前序基线或恢复未通过，或实验被取消 |
| S09 | — | — | — | — | — | 未执行：前序基线或恢复未通过，或实验被取消 |
| S10 | — | — | — | — | — | 未执行：前序基线或恢复未通过，或实验被取消 |
| S13 | — | — | — | — | — | 未执行：前序基线或恢复未通过，或实验被取消 |

### 扩容后连续验收第一轮：11 项通过

源码 0cb75b3，单元代码与 a54dc18 相同；仅增加 Redis 容量和记录说明。健康基线目标 200 QPS、10000 个活跃商品、64 工作者，各场景按定义临时调整流量，完整业务表仍有五十万商品。扩容后准备 healthy 用时 70.133 秒。第一轮开始 Unix 时间 1792914056.7515154，结束 1792916735.2723825，整轮命令约 2678.521 秒。最晚于 111.032 秒观察到症状和相关告警，最长恢复健康 134.967 秒，十一项全部通过。

第一轮退出成功后，脚本在 1792916735.286354 启动第二轮；中间没有补库存、清键或修改实验配置。第一轮本身还不能算连续两遍完成。完整输出 selftest-capacity-1.txt；266 次过程采样没有采集错误，见 selftest-capacity-1-metrics.jsonl。汇总按活动记录出现至下一场注入之前统计，包含恢复和健康观察窗口。

S04 窗口最大分钟回源速率 124.241/s，同键重叠计数速率 101.111/s，行锁等待最大值 0；唯一 firing 告警是 CacheLoadsHigh。S13 同时采样的 RPC/HTTP 比最大约 8.699，额外重试速率最大 941.225/s，库存 RPC 速率最大 1298.231/s。各项最大值未必来自同一时刻，不能拼成一条虚构样本；也不能把滑动窗口速率比当作每个 HTTP 的确切重试次数。

第一次临时读取 S09 profile 已晚于原版本恢复，得到总任务 109、目标发送堆栈 0，保留 S09-round1-profile-1.txt，不作为缺陷堆栈证据。第二轮另设只读采样，等待确认缺陷版本仍在运行再取两份相隔二十秒的快照。

| 场景 | 事前健康 | 注入耗时 | 症状及告警（自注入起） | 恢复健康（含恢复动作） | 告警 | 结果 |
|---|---:|---:|---:|---:|---|---|
| S01 | 1m55.176s | 30ms | 50.061s | 2m12.973s | InventoryLatencyHigh | 通过 |
| S02 | 1m0.125s | 30ms | 45.143s | 2m14.967s | OrderListLatencyHigh | 通过 |
| S03 | 1m0.115s | 30.988s | 1m51.032s | 1m26.21s | DatabasePoolWaiting | 通过 |
| S04 | 1m0.115s | 54.309s | 54.311s | 1m56.602s | CacheLoadsHigh | 通过 |
| S05 | 1m0.126s | 30.288s | 1m15.312s | 1m36.169s | CacheLoadsHigh | 通过 |
| S06 | 1m0.118s | 535ms | 45.557s | 1m21.019s | ServiceUnavailable | 通过 |
| S07 | 1m0.119s | 6ms | 1m30.051s | 1m55.183s | HTTPErrorRateHigh,InventoryCallFailures | 通过 |
| S08 | 1m0.116s | 5ms | 45.026s | 2m5.231s | RowLockWaiting | 通过 |
| S09 | 1m0.122s | 6.357s | 1m21.394s | 1m36.283s | GoroutinesGrowing | 通过 |
| S10 | 1m0.121s | 6ms | 50.032s | 2m5.19s | InventoryLatencyHigh | 通过 |
| S13 | 1m0.12s | 32.847s | 1m27.88s | 1m56.344s | InventoryRequestsAmplified | 通过 |

### S09 第二轮的两份实际堆栈

第二轮 S09 期间，先核对 app_build_info 的版本为 20261024.4，再抓取两次 `GET http://127.0.0.1:6061/debug/pprof/goroutine?debug=1`。时间为 2026-10-25T08:58:45.485018+00:00 和 2026-10-25T08:59:05.609764+00:00，间隔 20.124746 秒。请求在原 selftest 流量下执行，没有另加业务流量。

| 快照 | goroutine 总数 | deliverResponse 发送堆栈数量 |
|---|---:|---:|
| 第一份 | 1045 | 938 |
| 第二份 | 5405 | 5299 |

两份聚合堆栈都指向 `internal/shop/app/orderservice/response_async.go:29` 的通道发送。原始文件 S09-round2-profile-1.txt、S09-round2-profile-2.txt 和 S09-round2-profiles.json 保留在本机临时目录 opsdoctor-phase04。总数增长不能独自说明泄漏；相同发送位置持续积累、源码没有对应接收方，才把观察和原因连接起来。第一轮恢复后才抓到的正常快照仍保留，不拿它作为缺陷现场。

### 扩容后连续验收第二轮：11 项通过

源码 0cb75b3，第一轮退出成功后立即进入第二轮，中间没有补库存、清键、改代码或改配置。第二轮开始 Unix 时间 1792916735.3808055，结束 1792919421.6051435，整轮 2686.224 秒；266 次过程采样无错误。原始输出 selftest-capacity-2.txt 与 selftest-capacity-2-metrics.jsonl。两轮共 22/22 通过，症状及相关告警最长 111.032 秒，恢复健康最长 141.182 秒。两个计时都包含相应动作；恢复还包含连续一分钟健康观察。

| 场景 | 事前健康 | 注入耗时 | 症状及告警（自注入起） | 恢复健康（含恢复动作） | 告警 | 结果 |
|---|---:|---:|---:|---:|---|---|
| S01 | 1m0.113s | 67ms | 45.096s | 2m15.953s | InventoryLatencyHigh | 通过 |
| S02 | 1m0.116s | 48ms | 55.094s | 2m21.182s | OrderListLatencyHigh | 通过 |
| S03 | 1m0.125s | 29.958s | 1m14.98s | 1m56.263s | DatabasePoolWaiting | 通过 |
| S04 | 1m0.113s | 56.979s | 56.981s | 2m1.319s | CacheLoadsHigh | 通过 |
| S05 | 1m0.116s | 34.112s | 1m24.139s | 1m36.088s | CacheLoadsHigh | 通过 |
| S06 | 1m0.116s | 472ms | 45.494s | 1m21.039s | ServiceUnavailable | 通过 |
| S07 | 1m0.137s | 4ms | 1m30.051s | 2m5.198s | HTTPErrorRateHigh,InventoryCallFailures | 通过 |
| S08 | 1m0.119s | 3ms | 55.031s | 1m55.224s | RowLockWaiting | 通过 |
| S09 | 1m0.113s | 6.158s | 1m21.197s | 1m46.324s | GoroutinesGrowing | 通过 |
| S10 | 1m0.122s | 6ms | 55.035s | 2m15.225s | InventoryLatencyHigh | 通过 |
| S13 | 1m0.122s | 30.41s | 1m30.445s | 1m56.26s | InventoryRequestsAmplified | 通过 |

两轮 S04 各有 23 次过程采样，窗口包含恢复和下一场之前的健康观察，唯一 firing 告警都是 CacheLoadsHigh。分钟回源速率峰值分别 124.241/s、121.159/s；同键重叠回源速率峰值 101.111/s、108.170/s。行锁等待速率峰值分别 0、0.198312ms/s，均未触发 RowLockWaiting。第二轮包含恢复普通流量后的少量等待，所以不能把整个窗口写成绝对零；正式查询对照的两组行锁增量仍都是零。

两轮 S13 的额外重试速率峰值分别 941.225/s、1092.763/s，库存服务实际 RPC 速率峰值分别 1298.231/s、1283.295/s。同一采样点的库存 RPC / 入口 HTTP 速率比最大为 8.699、8.192。各列峰值可能在不同时刻；这里的比值从每个采样点分别计算，再取最大值。第二轮也出现了 DatabasePoolWaiting，表明放大后的下游请求同时带来连接池等待。相关告警仍以 InventoryRequestsAmplified 判定，不能仅凭配置上限声称每个请求都重试十次。

### 最终有流量的 18 容器内存快照与环境收尾

2026-10-25 两轮验收结束后，在暂停流量前于 2026-10-25T09:10:35.809618+00:00 记录 docker stats。按显示值换算相加约 3750.340 MiB（3.66 GiB），18 个容器配置上限共 6976 MiB（6.8125 GiB）。健康基线目标为 200 QPS，此时 Prometheus 一分钟 HTTP 速率为 220.361/s；配置目标和观测值分别记录。这是单次快照，不是峰值，也不包含整个 WSL/Windows 的开销。MySQL 约 1.165/1.5 GiB、Jaeger 380.4/1024 MiB、Redis 238.2/768 MiB。

| 容器 | 实际内存 | 上限 |
|---|---:|---:|
| alertmanager | 30.67MiB | 64MiB |
| alertsink | 19.59MiB | 64MiB |
| alloy | 169.9MiB | 256MiB |
| grafana | 146.8MiB | 256MiB |
| inventory-service | 15.47MiB | 128MiB |
| jaeger | 380.4MiB | 1GiB |
| kafka | 827.9MiB | 1GiB |
| kafka-exporter | 23MiB | 64MiB |
| loadgen | 17.33MiB | 128MiB |
| loki | 442MiB | 512MiB |
| mysqld-exporter | 16.28MiB | 64MiB |
| order-service | 19.84MiB | 256MiB |
| order-worker | 16.47MiB | 128MiB |
| prometheus | 154.5MiB | 512MiB |
| redis | 238.2MiB | 768MiB |
| redis-exporter | 17.89MiB | 64MiB |
| shop-mysql | 1.165GiB | 1.5GiB |
| toxiproxy | 21.14MiB | 128MiB |

所有容器运行正常，当前状态 OOMKilled 均为 false；三条代理均没有 toxic，消费积压为 0，firing 告警为空，活动实验文件已删除。原始文件 stats-final-healthy.jsonl、final-healthy-state.json。随后暂停 loadgen，保留全部容器和原数据卷。临时采集脚本误按 GET 的结构读取 POST 响应，暂停后的断言出现 KeyError；实际 POST 已成功，另行 GET 确认 qps=0，结果保存在 final-paused.json。没有重跑采样覆盖原有流量快照。

### 最终本机检查

两轮实验结束后只修改六处注释，澄清 Kafka 等待和后台批次的区别、仓储在途与真正 SQL 执行的区别，以及突发参数边界。没有改变运行逻辑。随后 gofmt、go vet、staticcheck、commentcheck、archcheck 全部通过；commentcheck 覆盖 179 份手写 Go 文件。再次用六个专用数据库及 Redis DB14/15 执行完整 go test -race ./... -count=1 -timeout=180s，125 个顶层测试通过，其中 16 个 Integration，0 失败、0 跳过。原始输出 static-final.json、all-race-final.jsonl。学习材料六组示例已实际运行并通过 gofmt。

### 首次 Linux CI 暴露空库授权顺序问题

提交 7bb074e 的 [CI 40179920077](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/40179920077) 在启动基础设施步骤失败。格式、模块、vet、staticcheck、注释与架构、无依赖 race、另一构建版本编译已通过；后续集成与业务冒烟未执行。MySQL 日志明确为 `005_chaos_user.sh` 的 GRANT 返回 `ERROR 1146 (42S02): Table 'shop.stock' doesn't exist`。空库初始化发生在 Go 服务迁移之前，而本机使用已有业务表，因此此前没有触发这个分支。

修正时保留两张表的原有权限边界。初始化脚本先创建账号；若两张业务表尚未出现，打印说明后结束自己的子 shell，让官方入口继续启动。服务健康后，README 和 CI 都显式再运行一次脚本，授予 stock、orders 的 SELECT、ALTER、INDEX。没有扩大到 shop.*，也没有把建业务表的职责移进初始化脚本。

用独立临时容器 opsdoctor-phase04-init-check 实测：MySQL 8.4.13，无网络、无宿主端口、独立新数据目录、内存上限 1536 MiB，只用演示口令。首次启动 7.454 秒成功，业务表出现前表级授权数为 0。随后仅在这个临时库建两张最小测试表，重跑脚本两遍，始终只有这两张表的六条授权，账号最多两个连接。原始结果 mysql-empty-init-result.json、日志 mysql-empty-init-logs.txt。测试后停止并自动移除自己的临时容器和临时卷，未重建用户的业务容器。两轮 selftest 所用业务代码和原账号权限均未改变，因此保留其结果；空库启动单独复验。

### 阶段 4 Linux CI 与推送

`bde7e7e` 已推送到 phase-04-chaos，[CI 40180713958](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/40180713958) 在 ubuntu-24.04、Go 1.27.1 上通过。静态检查、竞态、另一构建版本、真实依赖、50ms 网络代理注入与恢复、订单幂等和消息、完整观测接线均成功。CI 使用独立 small 数据；本机连续两轮故障验收见上文。随后只补验收链接与表格说明，不改运行逻辑。远端 main 核对仍为 0c96b0a，学习验收全部保持未勾选；完成后停在阶段 4。

<a id="phase-04-cache-rework"></a>

### 2026-10-25：缓存告警冷启动返工

Claude 验收反馈：缓存自然过期后恢复流量，旧 CacheLoadsHigh 因回源超过 50/s 立即触发，healthy 在故障注入之前等待约 2m30s。这个时间是验收反馈，不冒充本轮重新测得的数据。原规则没有 for。此前 P4-16 已说明扩大工作集会触发绝对回源门槛，但没有覆盖同一工作集停流后的正常填充。

本轮从 ebd4c2a 开始，只在 phase-04-chaos 返工。业务容器仍使用原有镜像，目标流量 200 QPS、活跃商品 10000、工作者 64；singleflight=true、TTL 抖动 60s，商品缓存保留 10–11 分钟。未修改业务表、缓存保留时间或场景的注入动作。

先从 Prometheus 重新查询前两轮实际数据，分母只取 GET /products/:id。查询窗口一分钟，查询步长十秒。S04 两轮回源比例峰值为 91.277% 和 93.362%；S05 为 51.162% 和 53.129%。将候选条件“回源比例 >35%，当前及两分钟前商品速率均 >20/s”代入这些历史样本，四段条件分别持续 50、40、50、40 秒，可覆盖 30 秒的 for。历史回放只用于选候选阈值，不能代替本轮实际 firing 验收。

候选表达式和校验保存在 deploy/prometheus/rules/shop.yml、cache_alert.test.yaml。先运行 promtool check rules 与 test rules，再通过 docker kill --signal=HUP opsdoctor-prometheus-1 热加载；这是让 Prometheus 重新读取规则，业务服务未重启。五组合成时间线全部通过，合成数据不计作性能实测。

原始文件保留在本机临时目录 `%TEMP%/opsdoctor-phase04/`：cache-rework-history.json、cache-rework-history-candidate.json、cache-rework-candidate-meta.json。前两份分别保存指标序列、候选条件满足的序列；meta 保存规则文件 SHA256 与加载时间。

首次全量竞态测试 124 个顶层测试通过、16 个 Integration 通过，TestRetryPolicy/剩余预算不够 未通过。源码里该用例使用 1ms 的真实截止时间，本次在首次 operation 前已经过期，实际调用为 0 次，而用例期待 1 次；输出保留在 cache-rework-race.jsonl。未改业务逻辑或测试门槛，原命令完整复跑后 125 个顶层测试全部通过，其中 16 个 Integration，0 失败、0 跳过（cache-rework-race-rerun.jsonl）。这个计时敏感性作为已知测试限制保留。gofmt、go vet、staticcheck、commentcheck（179 份手写 Go 文件）、archcheck 均通过，文件地图重新生成后内容相同。

#### 本轮正常流量观测

实际暂停约 673 秒，暂停采样的首尾间隔也已超过 660 秒，覆盖商品缓存的最长 10m+60s 保留时间。随后恢复目标 200 QPS，先观察五分钟冷缓存，再继续十五分钟。业务期共 120 次采样，0 次采集错误。完整告警历史另按一秒步长回查，结果为空。

| 时段 | 采样数 | 回源峰值 /s | 回源比例峰值 | 商品 P99 峰值估计 | 全库 SELECT 峰值 /s | 告警 |
|---|---:|---:|---:|---:|---:|---|
| 恢复后的前五分钟 | 30 | 71.172 | 86.640% | 24.754ms | 239.677 | 无 pending/firing |
| 随后十五分钟，包含正常过期 | 90 | 31.116 | 20.261% | 86.518ms | 206.611 | 无 pending/firing |

表内峰值取当前商品速率超过每秒 20 次的有效采样点；它们不一定发生在同一秒，不能拿两列峰值相除。恢复最初低流量时，两个独立计数的窗口比例也保存于原始记录，不拿它们校准比例门槛。检查告警历史时覆盖了全时段，没有过滤低流量时间。商品 P99 是直方图估算，全库 SELECT 包含库存和监控查询。

恢复两分钟附近的实测比例为 20.565%，已经低于 35%；这解释了为何历史流量窗口恢复后不会补报。正常下一轮到期的比例峰值为 20.261%，与历史 S05 的 51%–53% 有间隔。三十秒持续时间用于过滤短暂升高；它仍须通过接下来的故障实测。

业务窗口 Unix 时间为 1792923304.070175 至 1792924538.240879，共 1234.171 秒。原始文件为 cache-rework-pause-original.jsonl、cache-rework-cold-original.jsonl、cache-rework-healthy-original.jsonl、cache-rework-baseline-summary.json；文件名 original 指原有业务镜像，实际已加载候选告警规则。停流期间的两次全量测试会影响 MySQL 全实例行锁指标，因此暂停记录里有 RowLockWaiting；测试结束并消退后才恢复业务流量，业务观测窗口里没有这条告警。

规则文件 SHA256（包括最后一处纯注释措辞修正）：`cf119a4f274cf2d1d050114026f2788528acb861bf7f8c4c69b08d14ca52ca6e`。表达式在本轮观测中没有改变。Prometheus 仍按十秒抓取、十秒评估；生产只加载 shop.yml 中十二条规则，cache_alert.test.yaml 独立供 promtool 使用。

#### 第一次 S04：构建网络中断，未到故障验证

命令顺序为 `go run ./cmd/chaos healthy`、`inject S04`、失败后的 `restore S04`。事前 healthy 在 60.131 秒通过。inject 外层命令运行约 13.339 秒后退出 1，失败位置是发布配置前的镜像构建；缓存删除和突发动作尚未执行，因此没有执行 verify，不能把这一轮算成告警失败或通过。restore 退出 0，容器健康、singleflight 恢复原值、流量恢复 200 QPS，恢复记录已删除。

单独运行 `docker compose --env-file .env -f deploy/docker-compose.yml build order-service`，复现相同错误：构建器不能解析 auth.docker.io，获取 Docker Hub 的访问令牌失败（`failed to fetch oauth token`、`lookup auth.docker.io: no such host`）。这里只是访问固定基础镜像所需的仓库认证流程，不是项目凭据错误。宿主机 Resolve-DnsName 能正常解析；Docker 本机没有 golang:1.27.1-alpine3.24 和 alpine:3.24.2 的独立镜像，不能直接离线构建。没有改 Docker 全局网络设置，已请用户恢复 Docker Desktop 的访问。

原始步骤、时间和五秒采样保留在 cache-rework-S04-1-result.json、cache-rework-S04-1-metrics.jsonl；构建诊断保留在 cache-rework-build-diagnostic.log，输出已过滤本地凭据。候选规则和三分钟要求保持不变。

随后补查业务容器内 DNS 已可解析。再次原样执行 build order-service 成功，输出保留在 cache-rework-build-recheck.log；没有修改 Docker 全局配置或基础镜像版本。已告知用户无需再处理此前环境提示。等待期间短暂暂停压测，网络恢复后继续原验收流程。 用户随后明确确认：容器 DNS、docker pull 和构建都已通过，两个固定基础镜像已拉入本地缓存。用户要求后续构建网络失败先重试一次，仍失败再停止说明；未要求修改注入器的重试逻辑。

#### 新规则 S04 第一轮完整成功

网络复查后重新开始，原有恢复记录已清除，规则表达式未改。事前 healthy 实际 70.132s，通过后执行 inject S04、verify S04、restore S04、healthy，四步均退出 0。注入开始至正式症状与告警确认 96.507 秒；恢复命令开始至 healthy 结束共 81.609 秒，都小于三分钟。

从注入到恢复健康共 36 次五秒采样，0 采集错误。有效商品流量样本中的回源比例峰值为 90.595%，回源速率峰值 140.449/s，商品 P99 峰值估计 24.274ms，全实例 SELECT 峰值 349.757/s，行锁速率峰值 0ms/s。实际只有 CacheLoadsHigh firing，没有 RowLockWaiting。独立峰值不一定来自同一时刻。

原始步骤和秒级时间保留在 cache-rework-S04-2-result.json，过程在 cache-rework-S04-2-metrics.jsonl，汇总在 cache-rework-S04-2-summary.json。S04-1 是构建网络失败，S04-2 才是本轮新规则第一次完成的场景，不合并或删除失败轮。

#### 新规则 S05 第一轮完整成功

沿用 S04 最后的连续健康结果，直接运行 inject S05、verify S05、restore S05、healthy，四步均退出 0。注入到症状和正式告警共 109.791 秒，恢复动作加健康检查 76.411 秒。没有改变注入动作、35% 门槛、30s 持续时间或三分钟验收预算。

37 次五秒采样无采集错误。回源比例峰值 47.048%，回源速率峰值 71.585/s，商品 P99 峰值估计 77.799ms，全库 SELECT 峰值 257.420/s，行锁峰值 0ms/s。实际只触发 CacheLoadsHigh。原始文件前缀 cache-rework-S05-1，分别包含 result.json、metrics.jsonl、summary.json。

#### 新规则 S04 第二轮完整成功

四步命令全部退出 0。注入到正式告警 79.751 秒，恢复开始到 healthy 完成 96.494 秒。35 次五秒采样无采集错误，回源比例峰值 90.384%、回源速率 143.084/s、商品 P99 估计 24.180ms、全库 SELECT 359.525/s；行锁速率峰值 0ms/s，只有 CacheLoadsHigh firing。原始记录前缀 cache-rework-S04-3。

第一次 S04 更晚触发、较快结束恢复；第二次更早触发、等待告警窗口消退稍久。这两个时间都保留，不能只选更好看的数字。两次使用同一表达式和三十秒持续时间。

#### 新规则 S05 第二轮与定向复验汇总

第二轮 S05 四步均退出 0。注入到正式告警 108.156 秒，恢复到连续健康 86.359 秒。39 次五秒采样没有采集错误。回源比例峰值 49.441%、回源速率 75.381/s、商品 P99 估计 43.633ms、全库 SELECT 250.121/s，行锁峰值 0ms/s；只有 CacheLoadsHigh firing。文件前缀 cache-rework-S05-2。

| 完整流程 | 告警时间（自注入起） | 恢复加 healthy | 正式告警 | 结果 |
|---|---:|---:|---|---|
| S04 第一次成功 | 96.507s | 81.609s | CacheLoadsHigh | 四步均通过 |
| S05 第一次成功 | 109.791s | 76.411s | CacheLoadsHigh | 四步均通过 |
| S04 第二次成功 | 79.751s | 96.494s | CacheLoadsHigh | 四步均通过 |
| S05 第二次成功 | 108.156s | 86.359s | CacheLoadsHigh | 四步均通过 |

定向验证期间没有更改表达式、持续时间或注入动作。测试时 HEAD 为 ebd4c2a 加本次工作区规则，内容指纹见上文；实现随后保存为 23f34fc。完整 selftest 在 23f34fc 上额外开始一轮，文档更新不影响运行代码。它的结果另行记录，不把此前旧规则的两轮通过冒充新规则全量验收。

#### 新规则额外完整 selftest：23f34fc，十一项全部通过

实现提交 23f34fc，开始 Unix 时间 1792925985.1241057，结束 1792928687.931499，整轮 2702.807 秒。十一项全部通过；症状及正式告警最长 110.940 秒，恢复健康最长 137.628 秒。两个时间都包含相应动作，恢复还包含连续一分钟健康观察。268 次十秒采样没有采集错误。

| 场景 | 事前健康 | 注入耗时 | 症状及告警（自注入起） | 恢复健康（含恢复动作） | 告警 | 结果 |
|---|---:|---:|---:|---:|---|---|
| S01 | 1m0.128s | 68ms | 50.102s | 2m17.628s | InventoryLatencyHigh | 通过 |
| S02 | 1m0.122s | 29ms | 1m0.102s | 2m7.949s | OrderListLatencyHigh | 通过 |
| S03 | 1m0.118s | 25.897s | 1m50.94s | 2m1.305s | DatabasePoolWaiting | 通过 |
| S04 | 1m0.118s | 55.421s | 1m25.438s | 1m36.336s | CacheLoadsHigh | 通过 |
| S05 | 1m0.122s | 26.757s | 1m41.794s | 1m26.016s | CacheLoadsHigh | 通过 |
| S06 | 1m0.121s | 482ms | 45.506s | 1m15.957s | ServiceUnavailable | 通过 |
| S07 | 1m0.119s | 7ms | 1m10.044s | 2m0.191s | InventoryCallFailures | 通过 |
| S08 | 1m0.123s | 3ms | 1m0.036s | 1m55.227s | RowLockWaiting | 通过 |
| S09 | 1m0.121s | 6.292s | 1m21.335s | 1m46.129s | GoroutinesGrowing | 通过 |
| S10 | 1m0.118s | 5ms | 1m5.041s | 2m0.181s | InventoryLatencyHigh | 通过 |
| S13 | 1m0.256s | 27.456s | 1m17.485s | 2m6.421s | InventoryRequestsAmplified | 通过 |

本轮 S04 只有 CacheLoadsHigh，没有行锁告警。它和 S05 在两次定向实验之后，各完成了第三次完整流程；表达式、持续时间、注入方式和三分钟预算没有改变。完整原始表、指标和运行版本保存在 selftest-cache-rework-1.txt、同名前缀的 metrics.jsonl、meta.json 与 summary.json。文档编辑只改说明；其中 S05 把“慢查询涉及多个商品”改成回源调用链与到期时间，避免把未测出的变慢写成事实。

#### 返工结束状态与 CI

本轮完整 selftest 结束后，在 2026-10-25T11:45:01.241867+00:00 重新取快照：18 个容器正常、没有 OOM，三条代理均无故障规则，firing 告警为空。目标仍为 200 QPS，实测一分钟 HTTP 速率 209.713/s，消费积压 0。容器实际内存按显示值合计约 3866.184 MiB，配置上限合计 6976 MiB；这是单次快照，不是峰值。随后通过控制接口暂停压测并读回确认 QPS=0，保留容器和数据卷。

原始快照为 cache-rework-stats-final.jsonl、cache-rework-final-state.json、cache-rework-final-paused.json。

实现提交 23f34fc 已推送，[CI 40189124722](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/40189124722) 在固定 ubuntu-24.04、Go 1.27.1 上全部通过，包括五组 promtool 时间线、静态与竞态检查、真实依赖和观测接线。第一次 git push 曾遇到 github.com 临时解析失败，DNS 复查后原命令重试成功；没有改代理或远端配置。此后只补说明、结果表和验收链接，不改运行逻辑。

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

### DNS 返工 Linux CI 与收尾

[Linux CI 39992861971](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/39992861971) 验证源码提交 `5687c3386b5997c89efa0129a454981aa39ee22f`。2026-10-23 10:10:15 创建，10:16:31 完成（America/Los_Angeles），结论 success。固定 ubuntu-24.04 / Go 1.27.1，格式、tidy、vet、staticcheck、commentcheck、archcheck、无外部依赖 race、全新十七容器启动、真实 MySQL/Redis race、库存和订单消息冒烟全部通过。八个采集目标、三个数据源、十二面板定义、跨三个服务的 trace 和关联日志也通过。

本轮 CI 的 HTTP 冒烟持续 9.999990704 秒，目标 20 QPS，199 次请求、0 错误、0 dropped，实际 19.900018 QPS，P50=3ms、P99=21ms；商品/订单/列表为 141/43/15。随后核对 `ordered_quantity=445 sold_quantity=445 gap=0`。这是全新 small 数据上的十秒冒烟，不代替本机恢复实验，也不掩盖资源观察轮的业务拒绝和消息确认告警。

通过检查的 182 个非 Markdown 文件，按路径排序、统一 CRLF 为 LF，再逐文件取 SHA-256 并对“路径 + 制表符 + 文件哈希 + 换行”列表取 SHA-256，结果为 `9a8993f53e3922f89cbd0ffc96dc76e13e8d98e4343ab9619894a8d34c5bd2fa`。最终收尾只补 Markdown 中的 CI 证据、文件清单和进度，用 `[skip ci]` 提交；运行代码与通过 CI 的提交一致。

本机空闲配置已恢复 30m，loadgen 已暂停，保留实验产生的待核对占位和业务数据。阶段 3 返工工程验收完成，学习验收仍由用户本人勾选。分支保持 phase-03-observability，不合并 main，不开始阶段 4。

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


<a id="phase-06"></a>

## 阶段 6 原始记录

### 2026-10-27：开工与两项遗留修复

从 main f8ab9aea62802863b0f1569754aa12952780ec71 创建 phase-06-agent；origin/main 与本地一致。用户的阶段 0 学习材料表格和库存 app.go 两处 defer 修改原样保留，不加入提交。本机 .env 设置 LLM_PROVIDER=mock；尚未调用真实模型，模型花费为 0。

代码为 f8ab9ae 加本条所在的遗留修复提交。运行 `go test -race ./internal/doctor/source/... ./internal/doctor/tools ./internal/loadgen ./internal/chaos/... -count=1 -timeout=90s`，14 个包通过。MySQL/Redis 集成测试未注入环境变量，本轮是离线单元检查，不能算真实权限复验。相同范围 go vet、staticcheck 通过。全仓 commentcheck 通过（246 个非生成 Go 文件），archcheck 通过；格式检查没有输出。

TestSelectReasons 检查 6 种拒绝原因；TestExplainErrors 检查 3 种原文放行、2 种只给错误号，另检查中文截断和取消优先。TestSafeQueryError 检查错误能穿过注册表且普通后端错误仍隐藏。TestControlActiveBaseline 核对当前商品数、工作者数在暂停后仍返回。TestBaseline 的 4 组配置中，正常值可通过参数检查，商品数不符、工作者数不符、旧接口缺字段均在查询指标之前失败。

Docker 引擎未运行：dockerDesktopLinuxEngine 管道不存在。已请求用户启动；没有执行容器实验，没有把单元测试写成运行环境验收。


### 2026-10-27：Mock 接线、真实仓储与费用保护

工作树基于 5c723d3，以下均为未提交的阶段 6 实现；验收源码提交在收尾补充。第一次 doctor store race 集成出现两个真实问题：NULL report 直接扫描到 json.RawMessage 失败；告警脱敏后反序列化进浅拷贝结构，复用了调用者 map，导致并发数据竞争。分别改为先扫描 []byte、反序列化到全新 Notification。没有改测试断言来绕过问题。修复后六项医生仓储集成测试通过，包耗时 2.174s，skip=0。

Mock 页面接线：手动任务 2dea2bb9c95f07830acfb25a008dbf21 成功，7 步、2 轮模型、2 次查看工具，模型 mock-script-v1，费用 0。通过浏览器登录、选择任务，检查步骤与报告；截图 docs/reports/phase-06-mock.png。报告和顶部都有 Mock 标识。初版报告有重复的 Mock 提示，随后保留程序端统一标识，去掉脚本的重复前缀。

进一步核对账本发现，实际用量超过预留时原本只返回普通错误，不能阻止重启后的继续发送。新增 0010_budget_stop.sql：实际费用完整入账，停用原因持久化。TestIntegrationBudget 验证并发十次每次预留30微元，在100微元上限下只有三次成功；已知结算7微元与未知费用保留后合计67；最后一次实额35超过原预留30后合计72，账本停用，重开并提高上限到1000也不能再预留。所有数字是测试用微元，不是供应商账单。

本轮全仓 go test -race ./... -count=1 -timeout=180s：167 个顶层测试通过、25 个依赖测试跳过、47 个测试包通过。go vet、staticcheck 通过。commentcheck 首轮指出两段新测试缺内部讲解，补上判断理由后通过（289 个非生成 Go 文件），archcheck 通过。随后暂停 doctor、Alertmanager，加载全部11个TEST变量，真实 MySQL/Redis race 集成25个顶层测试/31个含子例通过，skip=0。Prometheus 配置、12条规则，以及cache_alert.test.yaml、goroutine_alert.test.yaml均通过。

Docker doctor 新镜像 opsdoctor/doctor:20261027.1，manifest list sha256:d9efb64ecd8528bc841875317555b53825b9e198893b15391af50ad32fe89c9d。重建时 .env 不进 Docker 上下文，医生不挂载实验目录。重建先保持 Mock。实验前用显式 stock restock 命令给1–10000号商品各加100000，10000项确认完成。第一次恢复流量误写 hot_percent，被控制接口以 INVALID_ARGUMENT 拒绝，未修改控制设置；改用契约字段 hot_sku_id=0。

### 2026-10-27：官方模型与验收前账户查询

用户明确指定 4.1 Flash，并授权把其提供的凭据写入已被 git 忽略的本机 .env；日志和文档不保存凭据。LLM_PROVIDER 保持 mock。UTC 2026-10-28 04:25:16 调用官方 GET /models、GET /user/balance：模型列表含 deepseek-flash，name 为 DeepSeek-V4.1-Flash；另有 deepseek-v4-pro，但没有选择它。余额接口 total_balance=1.66 CNY、granted_balance=0.72、topped_up_balance=0.93；子余额和总余额存在0.01的展示差，保留原值，不自行修正。此时 chat/completions 请求0次，探测生成费用0元。后续账单对照统一使用 total_balance，并说明接口小数精度限制。


### 2026-10-27：真实 S01 冒烟第一轮（未通过，保留）

基线10000商品、200 QPS、64工作者，先 healthy 65.171s 通过；其前一次误填控制字段导致的等待为145.176s，也已保留。首次付费诊断前切 openai，模型deepseek-flash、提示v6.1。04:34:14.885 UTC 注入 S01；verify确认症状和 InventoryLatencyHigh，注入到确认60.763s。自动创建 d4e228a791138f5453516fc4e1d1f923，开始04:35:12.258635，结束04:35:38.763073，执行26.504438s；30个持久化步骤、10轮模型、17次查看，输入97540/输出3245、缓存命中72704，保守费用¥0.221040。八万token是触发整理报告的阈值，最后机会仍会增加用量，不能宣称总实际token严格不超过八万。

模型已经查到 SHOW INDEX 只剩PRIMARY、EXPLAIN type=ALL/486234行、当前锁等待和库存超时。最后一轮报告的组件选 mysql:shop.stock，机制指向索引缺失；但 arguments JSON 没有完整闭合，manual建议也漏risk。程序拒绝（对象字段不合法），最终budget_exceeded、report=null。不能把未经校验的候选文字计为成功报告或正确率。页面实时显示了工具步骤和失败终态，截图phase-06-smoke-failed.png；截图时运行模式已切回Mock，详情仍是这次真实历史任务。

04:36:00.320 UTC 恢复完成，restore5.097s；随后healthy100.267s通过。此时同场故障只建立1个诊断，后续通知仍附着它。一次get_trace返回unavailable，模型随后用search_traces继续取证，失败工具没有中止循环。余额接口在04:36:00.450仍返回1.66，不能据此把真实请求记为免费；保守账本¥0.221040，最终再次核对供应商余额。

根据这次接线观察，通用提示升级v6.2：明确预算、证据充分即可提交、报告简短、合法JSON、manual也必须带risk；没有增加任何场景原因或针对特定场景的查询顺序。仍只给一次最后机会，不放宽报告校验。JSON语法错误增加字节位置；补缺risk与括号错误的测试。页面发现运行中轮数要到终态才更新，改为按已保存步骤实时汇总，并把实际模型名放到每个折叠步骤的标题里。修复后agent/httpapi两包race通过。


### 2026-10-27：S01 第二次冒烟与真实取消

v6.2、相同基线。04:40:08.932 UTC 注入，55.24s后verify通过。自动诊断6c16c8ad183e0133596f7ab7e18e31ff：04:41:02.225585开始、04:41:23.541409结束，21.315824s；28步、9轮、16次查看，输入97184/输出2843、命中69632，保守¥0.217112，succeeded。报告组件mysql:shop.stock与架构/场景一致；机制方向指出缺索引→扫描→锁和连接等待，但把GetStock普通SELECT与更新加锁混在一起，机制只能判部分正确。候选索引变更来源的推测没有证据，不能当作已证实事实。恢复3.687s。此轮为接线冒烟复验，不混入后面的正式11场统计。

页面真实取消：通过表单发起a8e5376525d16d93ac535204e3b94d17，再点击取消。开始04:42:26.016430，结束04:42:29.771304；11步、3轮、6次查看，输入12101/输出406、命中10368，保守¥0.120576（含未确认请求的完整预留），状态cancelled。日志：04:42:28.669711766开始第三次HTTP请求，04:42:29.732480614取消接口返回202，04:42:29.732635921记录“模型请求已中断”、reason=cancelled，04:42:29.784048109记录诊断结束。取消接口返回到中断日志约0.155ms，到结束日志约51.57ms；这不是从点击硬件时刻计时，不能把请求总耗时1062ms当作取消耗时。第三次请求没有重试。实际费用未知，预留没有退款。

浏览器观察还发现：手动任务先读到pending行时provider仍是迁移默认mock，随后running事件没有刷新初始模型，曾短暂误显示Mock。修复为没有模型时不显示历史Mock标记，running事件重新读取绑定后的模型，再接续SSE；每步实际模型仍独立保存。

### 正式验收前两场：保持 v6.2 不变

每场基线均为10000商品、目标200QPS、64工作者。S01于UTC04:44:37注入，04:45:32创建任务，04:45:51结束。程序接收有效报告，但模型选择链路标识inventory-service->mysql，与表级答案mysql:shop.stock不一致。机制中缺索引、扫描和池等待方向正确；查询的若干指标名不存在，部分证据语义与结论没有完全对应，判部分正确。正式这轮打开详情时已结束，只观察到完整回放；此前两次S01冒烟观察过运行中的步骤，不能把回放说成这轮实时观察。04:47:21恢复完成，04:49:36 healthy通过（135.264秒）。

S02于04:53:58注入，04:54:42创建5c143936176c1ce6bb948b6d68e75378，04:54:55结束。列表在运行中更新，打开详情时已转为结束。索引缺失、EXPLAIN全表扫描及filesort、调用链中的orders查询耗时互相支持，判组件一致、机制正确。报告没有证明是谁删的索引，明确列为未知；性能摘要中的DDL不包含执行人和确切时间。截图：[真实报告](reports/phase-06-real-report.png)。恢复动作10.464秒。S01第二次冒烟的恢复后healthy另为140.298秒。

内存快照19项（MiB）：doctor18.14、loadgen19.86、alloy160.2、prometheus115.1、alertmanager13.12、doctor-mysql223.5、order25.79、redis-exporter11.07、kafka-exporter13.06、worker13.46、mysqld-exporter9.594、inventory17.49、redis50.15、loki229.6、kafka585.5、shop-mysql1083.392、jaeger257.2、grafana139.5、toxiproxy27.5，总3013.226。原mysql显示1.058GiB，本表按1024换算，非峰值。学习材料三段Go示例分别输出true、17、缓冲已满后收到1；均退出0。

### 正式 S03、S04

S02恢复后healthy130.247秒通过。S03注入含构建32.554秒，从注入开始82.263秒verify通过；任务ebd2bfd3f2f60a104f7b5db2fde05467运行21.449647秒，33步、10轮、20次查看，106034/3464输入输出token，缓存命中87296，¥0.239780。页面在运行中打开，观察到第24步后继续增长并生成报告。组件inventory-service:database_pool一致；机制正确：发布时间、max_open由20到2、频繁等连接互相支持。报告中“可以排除数据库瓶颈”比瞬时样本能支持的结论更强，保留这个措辞局限。restore6.810秒，healthy115.277秒；故障组05:00:32已关闭，之后才注入下一场。

S04注入57.643秒，从注入开始87.476秒verify通过；任务0d0ddb7526863aa74169a6a0664e4244运行21.360059秒，35步、9轮、23次查看，111030/3797token，缓存命中87040，¥0.252436。页面观察到运行中的第32步，随后收到报告。组件order-service:product_cache一致。机制判部分正确：启动日志的singleflight关闭和同键并发回源指标确实支持击穿，但它把进程计数器归零当作Redis缓存变冷的证据，还声称重启清空进程内商品缓存。实际缓存存在Redis，计数器归零本身不能证明键被清掉；也没有充分解释实验中的反复过期与热点突发。保留原报告，不为这个场景改提示。恢复动作6.744秒。

余额中途查询的准确UTC为2026-10-28 04:59:41，total_balance=1.52（granted0.59、topped_up0.93），对照04:25起点1.66。此时医生保守累计1.155678；缓存折扣、报价口径、取消预留和账单更新延迟均可能造成差异，最终仍需再次查余额，不能把中途差额当最终费用。

### 正式 S05 首轮：无有效报告，保留失败

UTC05:06:06起注入，动作31.744秒，注入至verify106.570秒；任务a1d3a0b7d4ad7ff8751f149fcbc3d6e2在05:07:52开始、05:08:09结束，17.443835秒。31步、9轮、19次查看，92934/2986token，命中75776，¥0.209756。最后submit_report的arguments在第3478字节附近不是合法对象字段；返回具体错误，但这是预算触发后的最后一次报告机会，按规则budget_exceeded，report=null。不能给它记组件或机制正确，也不从未通过校验的草稿里抽一部分冒充报告。页面列表已显示预算结束，详情打开晚于终态，本轮不计实时详情观察。恢复6.686秒；等待healthy与整组resolved后才重跑，同样使用v6.2，不改校验和提示。S04恢复后的healthy60.156秒通过。

### S05 同版本完整重跑

首轮恢复后healthy60.170秒通过，故障组05:08:22已关闭。重跑从05:10:44注入，动作38.005秒，注入至verify107.229秒。任务44f6431bd89603c42fd3dd57c045081a在05:12:32开始、05:12:55结束，23.472022秒，32步、10轮、19次查看，105386/4047token，命中86400，¥0.243148。v6.2保持不变。本轮有效报告，页面在第29步仍运行时订阅，随后收到最终报告。

组件order-service:product_cache一致，机制部分正确。TTL抖动为0的启动日志与批量过期方向正确；但报告再次声称重启清空商品缓存预热，实际上键在Redis。它用cache_requests_total作分母也没有复现告警基于HTTP商品请求数的比例；缓存层内部二次检查会产生不同的计数口径，不能把差异直接归因于重启打断历史。Redis DB0还包含幂等记录，全部键的平均TTL不是商品TTL分布。这些都是报告的真实局限，没有据此把首轮结果改成通过。恢复动作7.342秒。

### 正式 S06：消费者退出

S05重跑恢复后的healthy125.245秒通过；整组05:13:02已恢复。S06注入0.738秒，从注入起55.892秒verify通过。任务5ea5c28f2c14427943448c9d528ebab8在05:16:27–05:16:44执行，17.326707秒，38步、9轮、26次查看，100988/3402token，命中79744，¥0.229192。组件order-worker一致，机制正确：退出信号与收尾日志、up=0、消费位点不推进、积压增长构成证据链；没有把未知的退出触发者硬说成人工停机。页面从第8步开始观察并实时增长，[运行截图](reports/phase-06-live.png)。restore0.955秒。

辅助脚本首次读取S06详情时收到401，未重新生成诊断；重新登录后读取原任务成功。页面此前的旧会话也到期，重新登录成功。未确定脚本这次401的原因，不把它解释成模型故障或漏计调用，也没有为了绕过而放宽JWT校验。

收尾检查发现预算指标只有布尔值，不能区分原因。按用户“页面和指标都显示原因”的要求补有限原因Gauge，并避免进程预留失败标志覆盖账本已持久化的具体停用原因。这不改变提示、模型请求或正式验收中的调度规则；后续完整Mock检查与集成会覆盖原因持久化。

### S06恢复期发现重复付费：当前工程验收未通过项

S06恢复后的healthy最终170.337秒通过，最后不满足的是条件6（消费者积压小于10）。继续核对数据库时发现，恢复阶段还创建了第二个真实诊断a7dbb28fb29899990b4ffdba803eb493，UTC05:18:08–05:18:30，22.094518秒，37步、9轮、25次查看，103912/3707token，命中83840，¥0.237480。原始输入表明两次groupKey都是`{}:{}`，不是按alertname分组漏改：第一份是ServiceUnavailable（startsAt05:16:12.419），第二份是ConsumerLagHigh（startsAt05:18:02.419）。消费者恢复后，ServiceUnavailable先resolved，原故障期被关闭；积压告警的持续时间门槛较长，稍后才firing，因而新建了任务。

这次实验实际产生2个诊断，合计¥0.466672，违反“一次故障只诊断一次”的验收目标。第二份报告把processed_events的主键误判成缺少唯一索引，并对积压恢复过程作了错误的瓶颈推断，组件和机制都不正确。不能因为第一份报告正确而隐藏恢复期的错误报告和费用。

UTC06:49:44余额复查total_balance=1.31（granted0.37、topped_up0.93），起点1.66，展示差额0.35；医生累计保守¥2.327690，含全部冒烟、取消、失败、重跑与额外恢复期诊断。两者口径不同，供应商展示也有分位精度/更新延迟。期间没有医生之外的付费生成探测。

发现后把本机LLM_PROVIDER与运行中doctor切回mock，暂停后续真实场景。按AGENTS第6节请求用户确认：是否将整组resolved改为先观察2分钟，期间再次firing并回原诊断，连续无新firing才结束。当前未实现这项待确认规则。S07–S10/S13尚未开始，工程验收保持未勾。

### 等待规则确认期间的离线与集成复查

预算原因指标补齐后，全仓race顶层169通过、25真实依赖跳过。vet、staticcheck通过。第一次commentcheck指出新增两个测试缺函数内部注释，补上状态切换和公开采集断言的理由后，290个非生成Go文件全部通过；archcheck通过。随后确认本机与运行中provider均为mock，暂停流量并等消费者追平，停止Alertmanager和doctor，再注入全部11个TEST变量运行真实集成：25顶层/31含子例通过、skip=0、exit=0。真实账本测试覆盖了total_limit和accounting_guard原因；未调用供应商生成接口。


#### 阶段 6：恢复观察期获批并实现（2026-10-28 UTC）

用户批准按系统设置观察期，shop 使用 3 分钟，计算为最长 for 120 秒 + 评估 10 秒 + group_wait 5 秒，下界 135 秒。新增外部配置契约测试、持久化观察截止时间和后续告警到达时间。targets 接口公开开放状态及预计期限；页面轻量刷新独立后续告警区。原 S06 两份诊断和额外费用不删。

定向 race 测试覆盖 target/store/httpapi/app/scenario 全部通过；commentcheck 294 文件、archcheck 通过。专用医生库 7 个集成测试通过、0 skip，包括可控时间推进的恢复观察、不同系统时长、重复 resolved 不延长、原报告及步骤不变、到期新任务、后续告警保留。真实 S06 复验尚未进行。本段记录的是实现和测试，不能当作真实场景成功。


#### S06 恢复观察期复验（2026-10-28 07:29 UTC）

07:29:16.850 注入 S06；verify 从注入计 54.017 秒，ServiceUnavailable 出现。任务 358396d5bcd586b73fe5ad793a34b9e3 从 07:29:52.241357 执行到 07:30:31.031602，共 38.790245 秒，29 步、9 轮、17 次查看，输入 98832 / 输出 3186 / 命中 76416，保守费用 ¥0.223152，提示 v6.2 未改。报告组件 order-service:kafka_producer 不符合 S06 标准。恢复流量后出现了 OrderLatencyHigh（activeAt 07:28:42，firing 07:29:42），它与 S06 通知合组，模型把历史消息发送慢当成主因，且把退出前的消费日志误当成当前仍运行。真实结果保留，不能算正确诊断。注入前 healthy 60.235 秒通过，但该延迟随后才满足 for，说明还需观察 pending，不能只看当时 firing 为空。

07:31:26.105 完成恢复（动作 0.948 秒）。ServiceUnavailable 于 07:31:32.419 恢复，ConsumerLagHigh 在 07:31:42.419 才 firing，07:31:51.365687 作为恢复期后续告警并入原 ID。07:32:02.419 积压告警恢复；接口随后给出 recovery_until=07:35:18.835198。页面单独显示后续告警及时间，截图 phase-06-recovery.png；报告仍为原 29 步、原 ¥0.223152。healthy 110.245 秒通过，最后等待的是 Kafka lag。07:34 核对新增诊断数为 1，继续等接口开放期结束后才开始下一场。

验收脚本两次遇到刚签发 JWT 随后 401。安全地仅打印 iat/exp 与响应 Date 核对：同令牌首个响应 Date 为 07:32:11，下一秒响应 Date 却为 07:32:03，Linux 环境时钟短暂向后跳约九秒，签发时间因此处于服务端未来。没有修改 JWT 校验或打印令牌；本地辅助脚本对 401 最多等待同一个令牌 15 秒，不通过重新创建诊断绕过。这个环境现象不能算业务鉴权绕过。


S06 复验观察期在 07:35:27.288 UTC 的 targets 响应中转为 open=false（预计期限 07:35:18.835，采样受本机时钟差异影响）。最终 1 个诊断，29 步、¥0.223152 未增加。整组恢复和观察期结束后才开始 S07。

#### 正式 S07（v6.2）

07:36:17.610 UTC 注入链路延迟；注入至 verify 100.151 秒，两条症状告警出现。任务 41af00e807d3dcac8b908246dd37fbed 从 07:37:48.090860 到 07:38:10.313939，22.223079 秒，28 步、9 轮、16 次查看；输入 97558 / 输出 2714 / 缓存命中 76928，保守 ¥0.216828。合法报告组件 order-service->inventory-service 与标准一致，机制部分正确：对照客户端失败与库存服务端毫秒级成功，定位调用链路；但没有确认网络响应延迟，且误称指标没有状态码标签（实际该标签存在，只是查询没有正确取到）。不能把未知原因算成完全正确。

页面列表看到了 running，打开详情时已到 succeeded，因此这轮只证明最终报告可查看，后面补做实时详情观察，首轮结果仍保留。07:38:51.135 恢复，动作 0.016 秒；继续 healthy 与接口等待。当前账本累计 ¥2.767670。


S07 healthy 于 07:40:51.345 UTC 通过，用时 110.200 秒，最后等待 HTTPErrorRateHigh。targets 在 07:42:41.985 返回 open=false，接口等待总计 230.674 秒；同场新增诊断核对为 1。07:40 有流量时 19 个容器 docker stats 快照合计 3760.16 MiB（3.672031 GiB），doctor 14.33/256 MiB、doctor-mysql 248.7/512 MiB；各容器分别为 order-service 25.47、inventory-service 17.56、loadgen 19.41、alloy 165.1、prometheus 128.7、alertmanager 13.37、redis-exporter 10.39、kafka-exporter 14.84、order-worker 15.14、mysqld-exporter 10.14、redis 178.8、loki 353.4、kafka 802.5、shop-mysql 1.14 GiB、jaeger 335.2、grafana 212.3、toxiproxy 27.45 MiB。是单次展示精度快照，非峰值，低于 4.5 GiB 暂停线。

#### 正式 S08（v6.2）

07:46:07.260 UTC 注入热点 400 QPS；50.255 秒确认 InventoryLatencyHigh 和 RowLockWaiting。任务 ed6c0418fcbc4e6b67c9ce3103aaaa6f 从 07:46:49.147041 到 07:47:14.064772，24.917731 秒；33 步、9 轮、21 次查看，输入 111392 / 输出 3254 / 缓存命中 85888，保守 ¥0.248816。页面在 running 时观察到 5 轮模型、14 次查看，之后报告提交，实时链路通过。

组件 mysql:shop.stock 与标准一致。机制正确：行锁等待集中 stock 的 uk_stock_sku，索引与 const 执行计划仍正常，Deduct 的慢 span 落在 UPDATE，连接被占用后传导为连接池排队。报告保留未取得 sku 流量分布的未知项，没有声称查到了具体商品号。不过正文把约 6495390→7502348 毫秒误写为 6.50→7.50 秒，数量级错误；根因方向正确不代表报告全部数字正确。07:47:42.317 恢复负载，动作 0.021 秒。当前累计保守 ¥3.016486。


恢复观察期提交 8f2db3d 的固定 Linux CI 40651268284 全部成功；包括格式、静态检查、注释与架构、race、真实 MySQL/Redis 集成、Compose 接线和 Mock/SSE 冒烟。没有在 CI 使用真实模型。


S08 healthy 在 07:49:52.690 UTC 通过，用时 120.261 秒，最后等待 InventoryLatencyHigh、DatabasePoolWaiting、RowLockWaiting；新增任务数 1，继续按 targets 状态等待下一场。


S08 的 targets 在 07:51:49.114 UTC 返回 open=false，等待 246.601 秒后才进入 S09。S09 开始前再次查询 open=false，07:52:04.945 完成部署故障版本，动作 6.734 秒。


#### 正式 S09 首轮失败（v6.2）

07:52:04.945 UTC 部署完成；从注入开始到症状和告警确认 163.347 秒，仍在三分钟内。任务 5138450c2d2d350ee283fddef6d89576 从 07:54:30.333431 到 07:55:02.934288，32.600857 秒；28 步、10 轮、15 次查看，输入 104143 / 输出 2958 / 缓存命中 83200，保守 ¥0.231950。页面运行时看到 7 轮模型、12 次查看，之后 budget_exceeded，report=null。

程序返回了具体校验错误：最终 submit_report 的 JSON 在第 3555 字节附近对象字段不合法。候选文本中多条 manual 建议缺少 risk，结构也没有正确闭合。虽然它查到了 deliverResponse 大量堆积并选择 response_processing，但候选不是合法报告，不计入正确诊断。保持 v6.2，不修改 schema 或偷偷替模型修正文。07:55:14.412 完成恢复，6.404 秒；继续 healthy 和 targets 等待。累计保守 ¥3.248436。


S09 首轮 healthy 于 07:56:42.838 UTC 通过，60.139 秒；同场 1 个诊断。下一场前 07:58:28.376 的 targets 已确认 open=false。旧等待进程随后在 07:58:35.626 也读到关闭，等待 200.580 秒；两次请求时间差不影响先查关闭再注入的顺序。

#### 正式 S10（v6.2）

07:58:28.633 UTC 注入库存到数据库链路 200ms 延迟；48.971 秒确认 InventoryLatencyHigh。任务 a232b3dfd2628e2f24fe29862b69fd3c 从 07:59:10.719261 到 07:59:30.836838，20.117577 秒；32 步、11 轮、18 次查看，输入 100571 / 输出 2529 / 命中 86016，保守 ¥0.221374。页面运行中已看到 10 轮、18 次查看，随后报告成功保存。

报告组件 inventory-service:database_pool 与标准 inventory-service->mysql 不一致，机制错误。它确实看到 prepare 和 query/exec 各约 201ms，也看到池到 20 和等待增加，却把网络往返耗时解释为取连接等待。连接池排队是后果，不是本次注入原因。报告还过强地以无锁等待排除数据库端瓶颈，并建议扩池；建议仅保存、没有执行。保留这次误判。07:59:45.514 恢复，动作 0.017 秒。累计保守 ¥3.469810。


S10 healthy 在 08:01:48.283 UTC 通过，65.160 秒，最后等待健康条件 5；故障观察期预计到 08:03:40.691，继续查询接口，不据此提前开始下一场。


S10 的恢复观察期在 08:03:48.518 UTC 返回 open=false，等待 242.804 秒；S13 前独立查询在 08:03:47.835 已确认关闭，因此没有在上一场开放时注入。

#### 正式 S13（v6.2）

08:04:27.710 UTC 注入完成，动作 39.736 秒；从注入开始 87.780 秒确认 InventoryRequestsAmplified。任务 b077145b51efc6f4b9b0c8f32f85dfb1 从 08:05:11.223600 到 08:05:36.875353，25.651753 秒；34 步、11 轮、20 次查看，输入 101780 / 输出 3507 / 命中 87168，保守 ¥0.231616。

报告组件 inventory-service:database_pool 与标准 order-service:inventory_client 不一致。机制部分正确：实测重试约 532 次/秒、库存 GetStock 约 675 次/秒，识别请求被重试放大；但把此前时窗里的连接池占满与熔断记录当作当前原因，未核对本轮单次 50ms、响应延迟 80ms、重试 10 次、关闭退避与熔断的生效配置，还误称系统正在恢复。症状没有在报告生成时恢复，08:10:48.018 才执行 restore（6.338 秒）。当前保守累计 ¥3.701426。

S13 自动诊断发生时会话被其他项目的截图打断，恢复本任务时已经结束，未声称看到了这次实时详情；将单独补充观察。截图后来被用户明确指认为其他对话，未处理其项目。第一次报告与费用保留。模型允许查询的历史窗口包含先前实验，程序没有清空历史或提供场景提示；时间归属判断失败是这次误判的重要因素。


S13 healthy 于 08:12:56.003 UTC 通过，60.143 秒；同场 1 个诊断。08:14:26.849 的 targets 返回 open=false，等待 218.634 秒。08:12:47 官方余额显示 ¥0.93（赠送余额已为 0），验收前 ¥1.66，展示差额 ¥0.73；同期医生保守累计 ¥3.701426。尚未作为最终账单。

#### S01 补充运行（v6.2，保留首轮评分）

08:14:27.317 UTC 再次注入；50.789 秒确认症状告警。任务 5301c7b70d43e9b32603b69e3b544259 从 08:15:18.120046 到 08:15:43.732829，25.612783 秒；32 步、8 轮、21 次查看，输入 101254 / 输出 3050 / 命中 75520，保守 ¥0.226908。本轮组件 mysql:shop.stock 一致、机制正确：索引列表缺 uk_stock_sku，EXPLAIN 约49.8万行全表扫描，结合当前行锁等待与超时。SQL历史能说明曾增删索引，不能据此断言是谁操作。

这次打开详情仍已结束，没有把列表 running 记录冒充实时详情。S01 的完整实时观察证据使用前面 v6.2 冒烟第二轮（P6-8，已有实时工具与报告）；本补充运行单列、不替换 P6-10 的首轮误判。08:17:15.585 完成恢复，2.258 秒。累计保守 ¥3.928334。


S01 补充运行 healthy 于 08:19:13.593 UTC 通过，110.201 秒，最后等待库存延迟、连接池等待与行锁等待；同场 1 个诊断。


S01 补充的故障在 08:21:18.669 UTC 关闭，等待 242.908 秒后才注入 S02。

#### S02 实时页面补充（v6.2）

08:21:30.041 UTC 注入，55.916 秒确认 OrderListLatencyHigh。任务 8ec2afd02db0d2340901b5de082f8cda 从 08:22:28.121471 到 08:22:54.018112，25.896641 秒；33 步、10 轮、20 次查看，输入 102152 / 输出 2802 / 命中 83584，保守 ¥0.226720。页面在刚 running 时显示 0 轮/0 查看，随后经 SSE 显示 9 轮/20 查看，最后报告保存，实时链路通过。

组件 mysql:shop.orders 一致。缺联合索引→全表扫描/filesort→列表变慢的主线正确；但把前一时段的库存扣减/补偿异常当作本轮订单服务池排队的后果，跨越了时间与连接池边界，因此这轮机制只记部分正确。首轮 S02 的正确结果不变，也不用补充结果替换它。08:23:24.830 恢复，16.266 秒。累计保守 ¥4.155054。

S02 实时补充同场仅 1 个诊断。首次 healthy 于 08:26:39.388 UTC 超时（180.033 秒），最后失败条件 6 是 Kafka 消费积压未连续低于 10，并非报告仍在运行。恢复观察期在 08:27:43.736 关闭，等待 258.621 秒。保留失败后原样重跑 healthy，于 08:31:15.351 通过（60.202 秒），没有改阈值或暂停流量冒充健康。08:31:33 再查关闭后开始下一场。


#### S07 实时页面补充（v6.2，失败保留）

08:31:33.882 UTC 注入；61.822 秒确认 InventoryCallFailures。任务 964aed838398dfc8ef0f7c6be0ccab11 从 08:32:38.038454 到 08:33:04.044769，26.006315 秒；31 步、8 轮、20 次查看，输入 95381 / 输出 3586 / 命中 70912，保守 ¥0.219450。页面在 running 时打开，看到 query_metrics、recent_changes、redis_info、search_logs 等步骤持续增加至第 28 步，随后收到 budget_exceeded 终态。

这次最终 JSON 不合法，多条 manual 建议缺少 risk，不能把候选推理当作报告。首轮 S07 的有效但部分正确报告保留，补跑失败单列。没有修改提示 v6.2 或放宽校验。08:33:23.386 完成 restore，0.013 秒；开始 healthy 和故障关闭等待。保守累计 ¥4.374504。

S07 实时补充 healthy 于 08:35:15.097 UTC 通过，100.180 秒，最后曾等待请求错误率回落；同场 1 个诊断。

S07 实时补充观察期于 08:37:24.005 UTC 返回 open=false，等待 229.015 秒；下一场注入前 Prometheus 没有 pending 或 firing 告警。

#### S13 实时页面补充（v6.2）

08:38:02.855 UTC 注入完成，动作 30.349 秒；从注入开始 80.376 秒确认 InventoryRequestsAmplified。任务 9e02224ca212d7844a55f7bb78e763cb 从 08:38:58.005762 到 08:39:20.078615，22.072853 秒；28 步、9 轮、16 次查看，输入 107366 / 输出 2625 / 命中 81536，保守 ¥0.235732。页面明确看到 running、实时连接已建立、7轮/14次查看，随后变成9轮/16次查看和有效报告。

组件 order-service:inventory_client 一致。机制记部分正确：同一链路多次约50ms的客户端超时、下游处理约1ms、调用量约8倍，支持过短单次预算与重试放大；但没取得本轮生效配置，没明确确认关闭退避与熔断、最多10次重试。把服务端处理正常概括成“不是下游真的慢”也遗漏响应链路80ms延迟。首轮S13的错误组件不替换。08:39:29.765 恢复，6.601秒；保守累计¥4.610236。

S13 实时补充 healthy 于 08:41:27.284 UTC 通过（85.182秒），同场1个诊断。观察期在08:43:29.092关闭（等待206.922秒），下一场前没有pending/firing告警。


#### S09 同版本重跑（v6.2）

08:43:44.041 UTC 注入完成，动作7.052秒；从注入开始156.929秒确认 GoroutinesGrowing。任务44e0c6c4ba276bc7ea5806b305ca4c0a从08:46:17.990720到08:46:43.524115，25.533395秒；28步、9轮、16次查看，输入95930/输出3363/命中77312，保守¥0.218764。页面正在运行时显示7轮/14次查看、实时连接已建立，随后保存完整报告。

组件order-service:response_processing一致，机制部分正确。它查到31748/31804个goroutine停在deliverResponse，并提出结果无人接收导致阻塞；但把08:38–08:40的上一场重试故障当成本次泄漏的触发条件，还把三分钟增长表述成约三十分钟。实际是08:43重新发布的S09版本，正常请求也会泄漏。报告没有确定channel send，仍列为推断。首轮JSON失败P6-27不替换，本轮格式成功不意味着整段因果正确。08:46:51.048恢复（6.770秒）。保守累计¥4.829000，之后不再安排真实模型调用。


#### 真实费用收尾与 Mock 恢复

S09 重跑 healthy 于08:48:28.241 UTC通过（60.143秒）。22个真实诊断的本地审计快照去重求和为¥4.829000，与正在运行医生的持久化账本一致；已知输入2101969、输出66492、缓存命中1669888 token。取消的未知用量不伪造token，费用仍保留预留。首轮11个主诊断¥2.438918，其余冒烟、重跑、补看页面、重复诊断和取消¥2.390082。

官方余额GET在08:48:02.426返回总余额¥0.63（赠送0，充值0.63）；验收前¥1.66，显示差额¥1.03。没有充值或其他付费探测；模型列表和余额GET不生成模型回答。医生使用输入未命中缓存单价保守记账，缓存优惠和取消预留解释了口径差异；余额只到分，不能当作精确到微元的账单。失败与误判没有从任一总额中扣除。

08:48修改本机.env为LLM_PROVIDER=mock，并执行compose up -d --no-build --no-deps --wait doctor。新容器健康，GET /api/v1/status 返回provider=mock、model=mock-script-v1；累计账本仍为4829000微元、上限20000000，真实历史保留。后续集成测试只在这个Mock状态运行。

S09 重跑同场仅1个诊断，故障开放期08:50:04.755 UTC关闭，等待156.596秒。

#### 收尾内存快照（200 QPS）

08:50:39.312 UTC，19容器总计4033.834MiB（3.939291GiB），低于4.5GiB暂停线。doctor已经切Mock；业务保持一万商品、200QPS、64工作者。它是单次采样，不声称覆盖整个实验的峰值。

| 容器 | 当前使用 / 上限 |
|---|---|
| opsdoctor-doctor-1 | 11.67MiB / 256MiB |
| opsdoctor-order-service-1 | 20.73MiB / 256MiB |
| opsdoctor-inventory-service-1 | 16.91MiB / 128MiB |
| opsdoctor-loadgen-1 | 19.48MiB / 128MiB |
| opsdoctor-alloy-1 | 187.4MiB / 256MiB |
| opsdoctor-prometheus-1 | 132.6MiB / 512MiB |
| opsdoctor-alertmanager-1 | 14.23MiB / 64MiB |
| opsdoctor-doctor-mysql-1 | 262.3MiB / 512MiB |
| opsdoctor-redis-exporter-1 | 11.02MiB / 64MiB |
| opsdoctor-kafka-exporter-1 | 17.82MiB / 64MiB |
| opsdoctor-order-worker-1 | 18.55MiB / 128MiB |
| opsdoctor-mysqld-exporter-1 | 10.29MiB / 64MiB |
| opsdoctor-redis-1 | 243.3MiB / 768MiB |
| opsdoctor-loki-1 | 436.1MiB / 512MiB |
| opsdoctor-kafka-1 | 825.4MiB / 1GiB |
| opsdoctor-shop-mysql-1 | 1.181GiB / 1.5GiB |
| opsdoctor-jaeger-1 | 406.1MiB / 1GiB |
| opsdoctor-grafana-1 | 161.9MiB / 256MiB |
| opsdoctor-toxiproxy-1 | 28.69MiB / 128MiB |


#### 最后一次完整检查

先运行gofmt、go vet、staticcheck、commentcheck、archcheck，再运行全仓go test -race ./... -count=1 -timeout=180s。格式无输出、所有静态检查通过，commentcheck覆盖294个非生成Go文件；race顶层172通过，26个依赖测试因该轮未注入TEST变量明确跳过，无失败。原始JSONL为opsdoctor-phase06-closing-unit.jsonl。

确认.env与运行中的医生均为Mock后，暂停流量，停止Alertmanager和doctor；只向测试子进程传递11项TEST配置。真实集成go test -race ... -run Integration -count=1 -timeout=90s：26个顶层、32个含子用例通过，skip=0、fail=0。原始JSONL为opsdoctor-phase06-closing-integration.jsonl。测试只用各自专用库；只读权限测试不修改业务行。随后promtool的cache_alert.test.yaml与goroutine_alert.test.yaml均SUCCESS，启动Mock医生，恢复200QPS，待健康后再恢复Alertmanager。

期间两条组合命令被自动审批检查拒绝，工具只返回blocked by policy，没有更具体原因。它们没有执行。改为白名单配置仅传单个测试子进程，以及分别执行规则检查、启动与流量恢复后全部成功；没有扩大权限或打印配置内容。


集成测试后恢复200QPS，healthy于08:54:17.048 UTC通过（70.145秒）；随后启动Alertmanager并确认健康，08:55:06故障状态open=false。页面重新加载显示Mock横幅；从页面发起0867ac749d25fbca7b38c088502bebff，2轮/2工具/7步骤，0token、0费用，累计账本仍¥4.829000。截图phase-06-final-mock.png同时展示横幅、报告标识与总账。没有删除真实记录。


#### 余额再次核对与提交后的 CI

文档提交7feb4a8已推送，CI 40660812228全部成功。gh run watch中途遇到GitHub TLS握手超时，这只是本机查询失败；随后直接读取run状态确认success，没有重跑或跳过CI。

09:06:35.215 UTC再次查询官方余额，结果从08:48的0.63更新到0.57；验收前1.66，因此较晚的展示差额为¥1.09。同期doctor保持mock，累计账本4829000微元不变，也没有新增真实诊断。0.06的变化可能来自供应商延后结算；未取得逐请求账单，不把这一推测写成已确认原因。保留08:48查询记录，并把汇报书的最终余额口径更新为这次较晚读数。


## 阶段 7：审批、断点恢复与安全（进行中）

2026-10-28，从 main 4e48448 创建 phase-07-reliability。完整读完计划书，用户两处未提交修改保留。开工时 Docker Linux 引擎未启动，用户启动后恢复；先用 Mock 修改预算、缓存结算和标题。尚未进行阶段 7 真实模型调用，未宣称任何验收通过。

首次定向测试发现旧 token 用例只报每轮十个输入 token，不会触及新的一万四千的测试边界。把该用例的虚构计量改为四千，准确触发给报告预留空间的分支，保留“仅一次查询、总两轮”的断言；复跑通过。新增测试另证超过旧八万后仍可调查、最终报告一次纠错、上下文硬限制和缓存三档价格。


### 阶段 6 遗留修复的第一轮检查

2026-10-29 04:07 UTC 前：gofmt 无输出，go vet、staticcheck、commentcheck（297 个非生成 Go 文件）、archcheck 和全仓 go test -race ./... -count=1 -timeout=180s 通过。本轮全仓未传 TEST 配置，依赖测试仍明确跳过；独立只传 TEST_DOCTOR_MYSQL_DSN 的医生仓储集成测试 8 个全部通过、0 skip，日志为本机临时文件 opsdoctor-phase07-store-integration.log。它不替代阶段最终的全套集成验收。

重新构建并启动 Mock 医生，迁移成功，/api/v1/status 显示新账户已花 0、上限 10000000 微元；阶段 6 的记录仍能查询。2026-10-29 04:07:12.382 UTC，官方余额接口返回 ¥0.56，作为阶段 7、8 这批真实调用之前的读数。此前阶段 6 最后记录为 ¥0.57；本阶段此时尚无付费请求，不把这 ¥0.01 的变化算成本阶段调用，原因未确认。


### S01 新预算真实复测：v7.0（2026-10-29 UTC）

源码预算修复提交 45df8b8，模型 deepseek-flash，非思考模式。基线为 10000 商品、200 QPS、64 工作者。04:08:34 healthy 首先通过，花 70.463 秒；原冷启动告警组到 04:10:37 确认关闭。04:10 后才切真实模型，按北京时间 12–14 点空闲时段使用输入未命中/命中/输出 ¥1/0.02/4 每百万 token。

04:11:36.747 注入 S01，动作 0.052 秒；04:12:33.195 verify 通过，从注入到症状确认 56.484 秒。自动任务 ccc705cb4930400d9ee1d4094dca8355，04:12:22.136489 开始，04:12:44.928821 结束。28 步、8 轮、17 次工具，输入 79278、输出 2691、缓存命中 64640 token，按每请求三档价格向上取整后 ¥0.026698。report_mode=voluntary、budget_reason 为空，模型主动提交，没有被预算要求结束。组件 mysql:shop.stock 与标准一致，指出 sku_id 索引缺失、全表扫描与更新锁等待；原始脱敏步骤和报告保存在本机 opsdoctor-phase07/budget-s01.json。04:18:44 查询，同场只建 1 个真实诊断。

04:14 第一次恢复命令漏了场景编号，CLI 返回“restore 需要场景编号”，没有执行动作；失败日志保留。04:15:06.827 使用 chaos restore S01 成功，4.149 秒。期间已经把本机和运行医生切回 Mock。04:18:06.950 首次 healthy 花 180.010 秒超时，条件 5（错误率低于 1%）不满足。不能把这轮恢复写成通过。

继续排查发现 Redis 达到 maxmemory 512 MiB、noeviction；04:19 附近新订单 503 约 35.45 次/秒，201 只有约 0.80 次/秒，消费 lag 为 0。INFO memory 的 used_memory 为 536892504 字节，maxmemory 为 536870912；INFO keyspace 有 1237567 个键且全部带过期时间。单独发送一笔新的幂等订单得到 503 UPSTREAM_UNAVAILABLE。这是恢复阶段的独立容量阻塞，不能继续用这份不健康环境验收 S10，也不能删幂等键或缩短承诺期限来掩盖。已暂停流量并向用户询问扩容；医生保持 Mock。S01 的预算结果保留，但这轮恢复有 Redis 容量干扰，不作为纯粹故障恢复性能对照。

余额 04:14:23 仍显示 ¥0.56，04:18:44.906 更新为 ¥0.53。以批前 ¥0.56 比较，显示扣费 ¥0.03；医生结算 ¥0.026698，差 ¥0.003302。余额只有分位精度，而且有延后更新，不能把两者差异说成精确的计费误差。本阶段其他付费调用此时为零。


### Redis 扩容复验和 MCP 客户端实测

第二次 healthy 于 04:21:54.184 UTC 再次超时（180.020 秒），仍为错误率条件，不删去这轮失败。用户随后明确批准 maxmemory 2 GiB、容器 2.5 GiB、整套实际占用暂停线 6 GiB。只重建 Redis，保留原卷、全部键、AOF、noeviction 与 24 小时幂等期限。恢复 200 QPS 后，04:29:22.923 healthy 通过，70.140 秒；04:29:23 确认故障组关闭。此前暂停流量，因此第一轮检查曾短暂不满足有流量条件。04:34:21 UTC 的 docker stats 显示 19 容器合计 3518.160 MiB（3.4357 GiB）；Redis 528.7 MiB/2.5 GiB、doctor 18.64 MiB、doctor-mysql 238.1 MiB。只是快照，不是峰值。

04:22:47 UTC，通过官方 Go MCP SDK v1.8.0 的真实 Streamable HTTP 客户端，连接本机 /mcp/shop。凭据只从进程环境进入请求头。列出 16 个只读工具，service_topology 返回成功，注册表结果大小 4086 字节。doctor-mysql 的 mcp_audits 中有 shop/service_topology/ok，数据库时间 04:22:37.293581 至 .296811；宿主机与容器时钟有差，不能跨时钟计算耗时。测试客户端源文件和日志保留在本机临时目录 opsdoctor-phase07-mcp-client.go/.log。未调用模型、费用为零。


### S10 新预算真实复测：v7.0（2026-10-29 UTC）

04:34:29.688 官方余额 ¥0.53。确认组已关闭后切真实模型，仍采用空闲时段 ¥1/0.02/4 每百万 token。04:34:36.788 inject S10，0.025 秒；04:35:21.927 verify 通过，45.041 秒。任务 34bb38e241a112e1e5b29bdba4ee7228，从 04:35:21.899166 到 04:35:52.894828，共 30.995662 秒。48 步、16 轮（十五轮调查加一次整理）、29 次工具，输入 188191、输出 3906、命中 159872 token，结算 ¥0.047148。报告有效，但 report_mode=budget、budget_reason=轮数达到上限，不能写成模型主动结束。

报告将组件写成 inventory-service:database_pool，标准为 inventory-service->mysql。它找到了池等待，却把等待当成原因；没有拿到具体 SQL 子 span，仍把连接池耗尽写成已证实根因。组件和机制都不正确，保留完整报告，不因预算变大而改评分。步骤和报告保存在本机 opsdoctor-phase07/budget-s10.json。04:42 查询，本场只创建一个诊断。

04:36 已将本机和运行中的医生切回 Mock。04:36:55.604 restore S10 成功，0.017 秒。04:39:16.511 healthy 通过，140.244 秒；开始时 DatabasePoolWaiting 仍在恢复窗口。04:41:23.621 确认故障组关闭，等待 126.934 秒。余额 04:36 仍为 ¥0.53，04:42:23.547 更新为 ¥0.48，本批显示扣费 ¥0.05，对比账本 ¥0.047148 相差 ¥0.002852。阶段7两场合计结算 ¥0.073846，较开工余额显示扣费 ¥0.08；余额到分且可能延后更新，不能把差额当成精确误差。


### OpenAI 协议与账本接线（2026-10-29 UTC）

用户改选 GPT-6.1 Terra，授权 $40 和单次 $0.30。官方模型列表核对成功；05:05:52.729 组织 Costs API 返回 403，项目 Key 无管理账单权限。没有索取或打印新凭据。美元实际扣费未取得，以下金额均是接口用量按官方价格结算的账本，不冒称供应商已扣金额。

05:06:27.063207–.189282，Mock/USD 接线任务 e9890d4c15cef61e51177273cfbe2780，9 步、2 轮、2 工具、费用零。05:07:01.015007–09.971109，Terra 冒烟任务 5f0c070f814b12bc4e6e3e03e6fe3c08，提示 v7.1，11 步、2 轮、4 工具、8.956102 秒；9664 输入、319 输出、4459 命中、5199 缓存写入 token，$0.008323。第一轮 $0.006137，第二轮 $0.002186；按普通/命中/写入/输出 $1/0.05/1.25/5 每百万 token 计算。工具是告警、近期变化、调用链和日志，报告主动结束并说明证据不足。随后切回 Mock。

### 审批、恢复和权限开发记录

先定义审批 Plan 和 Repository，再接准备校验、执行器、数据库事务、Agent 等待与回放，最后接页面和内部运维端口。迁移 0025 已在测试库和运行库应用，不改写迁移内容。曾在表格测试中引用不存在的 scriptedModel，改为本包 modelFunc；初次 Python 读中文文件沿用系统编码失败，改为明确 UTF-8。这些是开发失败，不计作运行功能通过。

增加审批过期事件，避免数据库改成 expired 后页面仍看起来待审批。续约的审查发现数据库持续不可达时不能一直相信旧租约，于是补本机保守到期计时器，并用 synctest 检查到期前仍可工作、到期后取消。最后报告请求中断也要消耗一次机会，恢复不能再得到新的两次整理机会；专门回放测试通过。

进程验收最初放在 doctor/store 下，被 archcheck 的 os/exec 禁令发现。将整组验收迁到 tools/recoverycheck，保留医生禁令。写到一半的用例初次批准失败，原因是测试错误地沿用了已经释放的执行租约；改成人的独立请求 context，与实际 HTTP 路径一致。再次运行只发生一次 POST，恢复通过 GET 确认真实状态。未放宽生产租约判断。

首次最终检查发现 store 的两个文件需 gofmt，三个新函数缺函数体里的判断说明；全部补齐。阶段最终 race 集成日志中，真实杀进程测试结果：查证恢复 2.1963937 秒、11 步、拓扑查询一次；换模型 2.1811142 秒、11 步、拓扑查询一次；待审批 0.397 秒、15 步；过时拒绝 2.5470667 秒；写到一半恢复 2.7394531 秒、GET 三次、POST 一次、原审批 executed。每项一次，均 Mock，专用库，两秒测试租约。

### S01：页面审批完整演示，v7.2（2026-10-29 UTC）

启动 Mock 医生、Alertmanager 与恢复流量的组合命令曾两次被客户端自动审批拒绝，只给出 blocked by policy。用户最终在仓库终端手动启动两个容器并恢复 200 QPS，返回确认 10000 商品、64 工作者。没有绕开客户端拦截。05:56:14.024100 healthy 通过，65.517 秒，随后才切真实 Terra。

05:56:58.034254 inject S01 成功（0.054 秒）；05:57:53.252028 verify 通过（55.060 秒）。自动诊断 8ae59dff9dd437c554d11bd9b5b31649，开始 05:57:39.270532，标题 DatabasePoolWaiting 等 2 条告警，prompt v7.2。模型第 3 轮提出创建 stock.sku_id 唯一索引，审批 53f418134159f9547a8859de596021fb；05:57:54.314754 进入 waiting_approval。

页面核对预览后，06:03:59.648905 点击批准，06:03:59.674151 接续运行。执行语句为 `CREATE UNIQUE INDEX idx_doctor_53f418134159f9547a88 ON stock (sku_id) ALGORITHM=INPLACE LOCK=NONE`（实际使用反引号引用标识符），耗时 6180 ms。模型随后查索引、执行计划、锁等待和指标，06:04:20.064695 主动交报告。38 步、5 轮、20 次工具，57573 输入、1326 输出、41595 缓存命中，$0.028682。总墙钟 400.794163 秒，其中约 365.36 秒等待审批；等待没有耗掉五分钟活动预算。组件 mysql:shop.stock 正确；报告明确当时告警仍响，未冒称全部恢复。

一分种直方图窗口：05:56:40 健康对照库存 P99 0.023865847s、下单 0.159426471s；05:58:22 故障库存 0.902994505s、下单 0.951691176s；06:05:00.734647 索引修复后库存 0.023144011s、下单 0.098410460s。行锁当前等待 0，三个池修改标志都是 0。都是单机单次快照，不当作长时间平均或峰值。

06:05:12.092990 healthy 在 0.024 秒内拒绝，明确指出 stock.idx_doctor_53f418134159f9547a88 残留并给人工撤销方法。06:05:23.309575 按审批单执行 DROP INDEX，0.229 秒；06:05:31.451010 chaos restore S01 成功，7.995 秒。06:07:41.936468 healthy 通过，125.266 秒，开始时 RowLockWaiting 尚在恢复窗口。06:09:45.401486 查 targets，故障组关闭。06:14:43 查任务，同场只有一个诊断。索引撤销前已把本机和运行医生切回 Mock。

OpenAI 本批 $0.028682，连同前面的协议冒烟合计 $0.037005。取消、失败记录与原人民币历史均未删除。页面证据保存在 reports/phase-07-approval.png。图中是“执行完成”和程序生成的 SQL，工具结果审计同时保存了 6180 ms。

### 内存、数据库授权与最终本机检查

06:14:43.159332，在 10000 商品、200 QPS、64 工作者下，19 个容器合计 4224.870 MiB（4.125850 GiB）。doctor 15.61/256 MiB，doctor-mysql 300.1/512 MiB，Redis 580.3/2560 MiB。低于用户批准的 6 GiB，属于单次快照。

SHOW GRANTS 实际为 diag_ddl：USAGE，加 shop.orders、processed_events、product_sales、products、stock 五张表的 INDEX；最大连接数 1。diag_ro：五张表 SELECT，原有 performance_schema 指定列 SELECT，额外只有 threads.PROCESSLIST_ID；最大连接数 3。授权原文随汇报书保存，不包含密码或密码摘要。INDEX 权限的集成测试同时核对全部 grants，只允许上述精确表，实际 SELECT/INSERT/UPDATE/DELETE 和 doctor 库查询均权限拒绝。

暂停流量和 doctor/Alertmanager 后注入十二个 TEST 变量，全套 `go test -race ... -run Integration -count=1 -timeout=90s -json`：37 个顶层通过，含子例 56 个通过，0 skip、0 fail。全仓离线 `go test -race ./... -count=1 -timeout=180s -json`：202 个顶层通过，含子例 712 个通过，41 个依赖用例跳过；这批跳过由独立真实集成补验，不能写成全仓没有跳过。vet、staticcheck、archcheck 通过，补完格式和函数内部说明后 commentcheck 337 个文件通过。原始 JSONL 保存在本机临时目录 opsdoctor-phase07，后续提交对应 CI 另记。

06:21:12.981836 UTC，集成后恢复流量的 healthy 通过，60.461 秒。随后启动 Alertmanager。最新医生 status 为 provider=mock、model=mock-script-v1，USD 账本 37005/40000000 微单位；页面同样显示 Mock 横幅，截图 reports/phase-07-final-mock.png。Prometheus 配置与 12 条告警规则有效，两份 promtool 规则测试均通过。

### 阶段 7 提交、CI 与最终状态

源码与完整学习材料提交 6a22664 已推送 phase-07-reliability；[CI 40831023926](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/40831023926) 全部通过（GitHub 最后更新时间 2026-10-29T06:55:02Z）。包括固定 ubuntu-24.04 上的格式、静态与项目规则、全仓竞态、真实依赖集成、库存/订单/消息/观测接线和 Mock 医生冒烟。没有真实模型调用。

提交后复核本机 LLM_PROVIDER=mock，运行 status 为 mock/mock-script-v1，美元账本仍 37005/40000000 微单位；活跃流量参数仍为 10000 商品、200 QPS、64 工作者。没有新付费诊断。用户在阶段 0 学习材料的表格修改与库存服务两处 defer 换行仍在工作区，未纳入任何提交。所有阶段的学习验收保持未勾选；不合并 main，不开始阶段 8。最后文档收尾提交只更新验收链接与进度。

## 阶段 8：评测体系（准备与试跑）

2026-10-29，用户明确同意进入阶段 8。fetch 后确认本地 main、origin/main 都是 4913685，阶段 7 已合并；从 main 创建 phase-08-eval。完整阅读 2128 行计划书，再核对本次补充要求。现有第 5.6 节仍写换模型是可选，按本次用户已批准的范围同步为 DeepSeek E1 对比。

开工只存在用户的两处修改：阶段 0 学习材料的表格对齐，以及库存服务的两个 defer 换行。前者 SHA256 为 DC0D4D5288D5D140E800F7E412680A1F0AB3AD0188026ECC1906FFD730EC306A；两者都不提交、不还原。

先查调用链：医生当前的模型、币种和费用账户都在进程装配时固定。采用用户允许的单独 DeepSeek E1 批次，切换前必须没有在途或待审批诊断。Terra E0–E3 在同一次注入上比较；跨模型对比使用各自注入，结果需说明这一差别。S04 是有限短时突发，S05 也会随着缓存填充自行缓解，所以记录每组开始距注入的时间，不能把后运行组看到的系统当成完全相同。

S12 可使用既有 X-Request-ID 业务路径：正常商品 GET 请求带有合法 ASCII 请求标识，现有访问日志记录它。攻击文字仅作为标识数据出现，不改业务表，不新增故障开关。实际可见性仍需通过 Loki 实测确认，不能只因 HTTP 请求成功就声称提示注入场景已验证。

#### 阶段 8：试跑前的真实检查

2026-10-29 UTC，先只启动两个 MySQL 和 Redis，在 Mock 下跑了专用集成检查：41 个顶层测试、含子用例 60 个通过，skip=0。新增检查包括评测暂停、唯一槽位、审批拒绝、token 事务和结果不可覆盖。随后全仓 race：214 个顶层非集成测试、含子用例 753 个通过；该轮刻意不传 TEST 变量，集成项另由前一轮覆盖。go vet、固定版本 staticcheck、commentcheck、archcheck 通过。

启动全部 19 个项目容器时医生仍为 Mock。08:41:14 UTC 的实际压测配置为 10000 商品、200 QPS、64 工作者；启动后约 57 秒的累计计数有 41 个错误和 663 个 dropped，属于冷启动原始记录，未当作健康基线。08:41:56 UTC 准备试跑批次 phase08-pilot-20261029，docker stats 总内存 2.669 GiB，低于 6 GiB 暂停线。美元账户试跑前已用 $0.037005；DeepSeek 余额查询为 ¥3.34，本阶段尚未调用评委或对比模型。

本轮健康检查在 08:42:24 UTC 通过，连续观察 60.119 秒。08:43:03 UTC 注入 S01，继续使用原场景的 verify 门槛，没有降低症状条件。试跑医生为 Terra，自动告警执行暂停，所有写审批由评测策略拒绝。

S01 verify 在 08:44:04 UTC 通过（注入后 61.795 秒）。试跑任务 a2e9fefc9a9d698e06205684d6ed721a，运行 24.415 秒、5 轮、15 次工具，输入 52358、输出 1149、命中 39387 token，USD 0.023928，voluntary。组件 mysql:shop.stock 精确一致。模型查看索引、执行计划、慢链、锁与池指标；主动保留“缺索引时间未知”等限制。提出一张 create_index 审批，被 eval-policy 拒绝，之后改为人工建议并交报告。08:45:59 UTC 已执行 chaos restore S01，原索引恢复，等待 healthy 和开放期。

S01 恢复后的 healthy 在 08:48:11 UTC 通过，检查耗时 100.167 秒；初期健康条件 5 尚未满足，原结果保留。医生接口在 08:50:10 UTC 返回 fault.open=false。下一次注入仍需等到距 08:45:59 恢复至少五分钟，不能只看告警关闭。

S03 在 08:51:38 UTC 完成注入，命令耗时 34.725 秒（包括重建库存服务）；08:52:30 UTC verify 通过，注入起 85.664 秒出现 DatabasePoolWaiting。任务 89745c351840721ead05803195606585，5 轮、19 工具，输入 50094、输出 1208、命中 36675 token，12.679 秒主动提交，USD 0.024647。组件 inventory-service:database_pool 一致。报告说明当前两条连接、此前采样为二十条，并指出平均每次等待只有约 2.7–4.5 毫秒，用户影响程度仍未知。调池审批被拒绝，未执行。08:54:33 UTC 完成恢复，命令耗时 6.220 秒。

S03 恢复后的 healthy 在 08:56:22 UTC 通过，耗时 70.159 秒，最初仍有 DatabasePoolWaiting。只读核对两次试跑的费用：美元账户由 37005 增至 85580 微美元，差额 48575 恰等于两份结果；人民币账户仍为 73846 微元。第一次审计 SELECT 把审批列写成 tool，MySQL 返回 1054，随后改用真实列 tool_name；这是查询脚本错误，没有修改数据库或产生模型调用。

S03 的故障开放期在 08:58:14 UTC 结束；下一次注入仍等到恢复后五分钟。两张审批 SQL 审计均为 rejected / eval-policy / executed_at=NULL；十次请求全部 ok，无未知结果预留。


#### 阶段 8：第三场失败与检查点收尾

S10 在 08:59:38 UTC 注入，09:00:34 verify 通过，注入起 55.394 秒。唯一任务 8b7baa52ff1294aa83fd1b599c55f5b3 首轮未取得响应，错误为“模型网络请求未完成，费用待核对”。1 轮、0 工具、无报告，保留 USD 0.053598 和 30590 token 预留；没有重新跑一次来替换失败。宿主机命令 monotonic 耗时 12.439 秒。数据库开始 09:01:29.285779、结束 09:01:27.286370 UTC，倒序 -1.999 秒，原因尚未定位。不能把它变成零再计算平均。只调整了导出展示：标记时间无效、用量未知、无报告；旧结果 JSON 和步骤保持原样。新增 TestMarkdownUnknown 后再次全仓 race：215 个顶层、含子用例 754 个通过；没有再次调用付费模型。

09:03:16 UTC 完成 restore S10。09:05:26 healthy 通过（70.155 秒，最初仍有库存延迟与池等待告警）；09:07:32 接口确认故障关闭；09:09:31 退出评测。此前已把 .env 和运行容器切回 Mock，接口实际返回 mock / mock-script-v1。仍是 10000 商品、200 QPS、64 工作者。

最终 SQL 审计：3 个 eval 任务，2 succeeded、1 failed；3 个自动告警捕获记录全部 cancelled。两张审批分别 create_index、set_db_pool_size，均 rejected / eval-policy / executed_at NULL。模型尝试 10 ok、1 network_error，已知输入 102452、输出 2357 token；账本增量 102173 微美元，其中未知预留 53598。美元账户累计 139178 微美元；人民币仍为 73846 微元。一次审计查询的 tokens_in 未限定表别名，返回 1052，补 a. 后读取成功；没有写数据。

DeepSeek 官方余额在 08:41:34 为 ¥3.34，09:04:37 为 ¥3.33；本阶段没有 DeepSeek 模型调用，人民币账本没有增量，¥0.01 差额未归因。不得声称这就是评委费，也不得把差额抹掉。OpenAI 项目 Key 无组织费用权限，没有再次无意义探测该接口。

原始 CLI/接口 JSON、完整步骤、单元与集成 JSONL 位于本机临时目录 opsdoctor-phase08；正式可复核摘要和任务 ID 已写进仓库报告与数据台账，完整诊断仍在 doctor 库。试跑版本、金额和时间估算见 phase-08-pilot.md。没有执行正式矩阵、没有付费评委、没有替用户打分。

<a id="phase08-preflight"></a>
#### 阶段 8：审阅后的网络阻塞与计时修复

Claude 已审阅手册，用户已确认试跑估算。先把 changes-and-safety 的两句话改为通用描述，版本为 runbooks-v1.1，正文哈希 d86e159673bf5351743b10ec737f4b52b7281b1cad525e4c09c17c4897790f54。缓存文章未改，正式版本尚未冻结。

网络检查在现有 doctor 容器中执行，命令为 `docker exec opsdoctor-doctor-1 /etc/doctor/targets/.networkcheck.out`。临时程序由本机 Go 1.27.1 用 GOOS=linux、GOARCH=amd64、CGO_ENABLED=0 编译，只依赖标准库。它与生产代码一样以 nil Transport 创建 http.Client，禁止重定向；Key 只从容器环境读取。每次请求以 context.WithTimeout 限六十秒，用 httptrace.GotFirstResponseByte 记录首字节，用 time.Since 计时。五秒串行间隔，总单调时长十分钟，无重试；只访问固定 https://api.openai.com/v1/models。没有修改真实客户端或切换出 Mock。

网络程序起止墙钟为 09:35:32.717455904 / 09:45:22.695297125 UTC，单调耗时 600003ms。109 次中 2 次失败：第 99 次墙钟开始 09:43:42.969373954，60002ms 后仍无首字节；第 100 次开始 09:44:42.969846219，整数耗时记 0ms，也无首字节。没有保存原始网络异常文字，因此不能声称已经定位 DNS、代理或上游原因。完整采样已保存为 reports/phase-08-network-preflight.jsonl。完成后删除两个临时二进制；没有把新的挂载或执行工具留在医生容器中。

检查期间源码读到 StartTask 用数据库时间，EndTask 用进程 Now。初次读时钟发现数据库领先宿主机约半秒，稍后五轮高精度采样又发现 doctor 与 doctor-mysql 都落后约十秒。网络样本相邻开始日期倒序 19 次。原始时钟采样包含宿主机请求前后边界和精度，保存在 reports/phase-08-clock-preflight.json。没有修改系统时间，没有把回拨原因猜成某个软件。

已新增迁移 0032，保存独立 duration_ms 与执行段数；同一 execute 内 time.Since 测量。统一诊断审计时间戳为数据库 UTC。旧结果不重写；跨进程接手或老记录缺少测量时留空。单调时长的 Mock 测试故意设置日期倒序；集成测试把可注入进程时钟设到 2000 年，仍应保存数据库日期。恢复测试证明末段不能冒充完整时长。

无响应重试最多额外两次，退避一秒、两秒；每次先独立 Reserve，旧未知预留不退。测试覆盖部分响应、取消、无余额与审计失败。第一次 commentcheck 发现新增调度测试缺内部说明，补上后通过。staticcheck 最初不在当前 PATH，随后使用已安装的 C:/Users/Nepenthe/go/bin/staticcheck.exe，通过；没有安装系统软件。一次 Go 测试因现有外部缓存目录权限失败，经授权使用缓存后通过。

最终本机检查：gofmt、vet、staticcheck、commentcheck 364 文件、archcheck 均通过。全仓 race 221 顶层 / 763 含子例通过；完整集成 42 顶层 / 63 含子例通过，全部十二项 TEST 配置，skip=0。完整测试 JSONL 仍在本机临时目录 opsdoctor-phase08。09:48:59 UTC 的接口确认 Mock、故障关闭、评测模式关闭、10000 商品 / 200 QPS / 64 工作者，USD 账本 139178 微美元不变。

按用户“失败率明显不为零先停下来”的要求，已请求用户检查 Docker 代理或网络。没有执行五组付费检查，没有调用评委，没有正式冻结或运行。一次补跑与 DeepSeek 配对等剩余实现尚未完成。当前运行镜像保持旧版 Mock，新增迁移仅在专用测试库应用；下一次继续前需重建医生。两处用户修改完整保留。

#### 阶段 8：用户校时后仍发现跳变

用户转述 Claude 查明 Windows 快约十秒、WSL NTP 校时冲突，并说明 OpenAI 依赖 v2rayN。按要求先从 doctor 容器检查六十次：命令先读 prev=date +%s，然后循环 sleep 1、读取 now、记录序号/now/now-prev。57 次为一秒，第 53–55 次分别为 0、+12、−10 秒。完整记录已转为 reports/phase-08-clock-stability.jsonl，没有删掉异常样本。

后续十分钟网络命令前的校验明确失败，因此 Go 网络探测二进制没有重新构建，网络请求没有发出。先前仅查看末几行都是一秒不能代表整段通过，检查器核对了全部六十行才作结论。已向用户说明仍跳变，并请求继续检查环境。

离线增加 runner/clock.go 和 clock_test.go，把开跑前检查接入 prepare、pilot，把持续守卫接入 waitDiagnosis。分段命令有限运行六十秒，按行发现异常，不等满一分钟。取消原因用 context.WithCancelCause 保留，数据库轮询被取消时也返回时钟原因；原有 defer 再请求取消医生诊断。固定样本和取消 Mock 测试通过，runner race 通过，全仓静态及规则检查通过，commentcheck 366 文件。未启动正式矩阵或付费流程。

此前提交 4ff94e5 的 CI 40853597449 全部通过，包含固定 Linux 环境的整套 Compose 和 Mock 验证。时钟守卫在它之后新增，将单独提交检查；没有用旧 CI 冒充新代码的结果。

### 阶段 8：末次校时后复验

两轮仓库时钟守卫共 120 个一秒步长全部通过。完整十分钟免费模型列表请求 120/120 成功，不加显式代理。时差对照与原失败见 [预检末节](reports/phase-08-preflight.md)。没有付费调用。

利用检查等待时间实现一次基础设施补跑的仓储边界及导出标记。首次专用库测试发现 INSERT SELECT 的 result_key 列名歧义，明确限定来源后五项 Eval 集成通过、0 skip。Mock race 与静态检查通过；正式矩阵、配对和评委仍需继续，不能视为阶段完成。


### 阶段 8：配对、评分和自动续跑的实现记录

校时与网络复验通过后继续离线实现，没有新增付费调用。将按进程固定的一套模型扩展为启动时登记的两套执行配置；每个任务保存账户和币种，费用预留再次核对，Mock 总开关不装配真实对比。五组共享一次故障开始时间和告警快照，顺序首次生成后保存。

实现评委时把真实观察与模型自述分开，发现输入证据步骤使用 Status=input_evidence，而不是 ToolName，已据此修正过滤。价格配置初稿把空节假日列表传给必须非空的 Reader.String，被测试拒绝，改为显式 none。新 S11 加入后，原“未知 S11”测试改为真正未知的 S14，保留拒绝未知场景的同一断言。

11:40 UTC 的检查点：Mock race 240 顶层 / 824 含子用例通过；十二项 TEST 的集成 47 顶层 / 68 含子用例通过，0 skip。staticcheck 随后指出错误变量 requestTimeout 应以 err 开头，改为 errRequestTimeout；不能把那次检查说成全部静态检查通过。

继续实现整轮状态持久化、恢复等待、长跑登录续期、结果页面、分层统计和盲评空表。11:57 的集成检查发现 0037 把两条 CREATE TABLE 放在同一迁移文件；现有迁移器禁止多语句，MySQL 返回 1064。该新迁移尚未生效，拆成 0037、0038，校准表另用 0039；保留失败日志，重跑验证。若注入中断且无法在原三分钟窗口确认症状，程序暂停，不重注入挑结果。

最近已提交检查点 afcafc9 的固定 Linux CI 40859602123 全部通过。上述新增实现还在本机验证，不能把旧 CI 冒充新实现的验证。阶段仍未完成，用户两处修改未动，学习验收未勾。

12:10 UTC 新 Mock 医生和订单服务重建健康，chaos healthy 用时 60.534 秒通过。浏览器实际登录后展开结果区域，旧 S10 失败和 $0.0536 费用仍显示，机制显示未评分、时长显示未测量，截图见 reports/phase-08-results-preview.png。没有替历史倒序时间补算新时长。

### 阶段 8：五组检查前发现微秒精度错误

12:20 UTC，批次 phase08-lastcheck-20261029 完成时钟预检和 healthy，注入 S01 后在 verifying 被恢复记录校验挡住。文件保存的时间是 12:20:02.015277600 UTC；DATETIME(6) 入库时向上取整为 .015278，而比较函数使用截断后的 .015277。两边只是精度处理不同，不能因此放宽身份比较。

先手工执行 chaos restore S01，3.260 秒成功，随后 healthy 75.264 秒通过。本机和运行中的医生切回 Mock。原批次保留 interrupted、verifying 和原始数据库时间；没有创建模型槽位，两个账户没有增加费用。修复轮次、结果槽位和诊断三个入库位置，使它们先统一为 UTC 并截断到微秒。集成用例故意输入 .015277600 和非 UTC 时区，验证读回值与恢复比较一致；48 顶层、69 含子例通过，0 skip。首次旧批次 finish 因新告警仍开放被拒绝，继续等待，不强行解除占用。

此前实现检查点 5329461 的 CI 40871357080 全部通过。这次检查是在正式冻结之前发现的实现问题，将另开最后检查批次；不修改旧记录，不把它算作模型可靠性失败或诊断误判。
