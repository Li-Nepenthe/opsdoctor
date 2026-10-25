# 实测记录

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
