# 面试复述提纲

这份提纲用于面试前用自己的话复述。先遮住要点回答，再打开出处核对，尤其要分清“代码提供了什么”“实验测到了什么”和“还没有验证什么”。它汇总各阶段必答问题，并补入学习材料中的追问与项目问题。阶段 9 已由独立医生在外部 Demo 完成五场景 E1/E2 正式批次，两组各有 5 份 normal 报告和 5 份有效评分，组件一致均为 3/5、机制均为 2 正确/2 部分/1 错误，证据覆盖均值均为 0.75（P9-77）。原试跑失败评分保留、不重评；正式结果与阶段 8 分开统计。小样本不能外推生产准确率，工程交付后的 Claude 验收和用户学习验收仍分别进行。

实测数字只引用 [数据台账](数据台账.md)，旁边写编号；超时、连接池等配置值应回到对应代码确认，不把配置当成性能结果。阶段 8 正式冻结始于 `5e5d5a1`，获准的基础设施修订为 `ca2ec68`、`89945fd`，提示 v8.0、手册 v1.1、评分 v1.1 保持不变，见台账 P8-38、P8-194。下文阶段 8 数字对应这套冻结版本，不是阶段 10 重新评测的成绩。

## 阶段 0：地基

### 0.1 Go 的 internal 目录有什么特殊作用？

- Go 编译器限制导入范围：只有位于 `internal` 父目录所覆盖树中的代码才能导入它。
- 它阻止仓库外部随便依赖实现细节，但不能隔开同仓库的 shop 与 doctor。
- 项目内部的依赖方向由 depguard 检查；archcheck 检查目录布局、标准答案访问和 Compose 边界，权限隔离则靠工具与账号。

出处：[depguard 配置](../.golangci.yml)、[archcheck](../tools/archcheck/main.go)、[阶段 0 汇报](reports/phase-00-foundation.md)。

### 0.2 为什么配置用环境变量，而不把配置文件打包进程序？

- 同一份构建可以在不同环境配置地址、超时和凭据，不用改源码重编译。
- 真实密码和 Key 不进入公开仓库，示例文件只给假值。
- 启动时先检查必填项、范围和组合关系，让错误尽早出现；环境变量本身也不能被日志泄露。

出处：[配置读取](../internal/platform/config/config.go)、[阶段 0 汇报](reports/phase-00-foundation.md)。

### 0.3 优雅停机的完整流程是什么？不做会怎样？

- 收到操作系统信号后通知入口停止接新工作，通过 context 取消后台任务。
- 正在处理的请求或消息在有限预算内收尾，再关闭服务器、客户端和数据库。
- 直接退出可能截断响应、留下不确定写入或使已消费消息重新投递；超时后的强制结束仍是最后兜底。

出处：[生命周期](../internal/platform/lifecycle/lifecycle.go)、[worker 入口](../internal/shop/app/orderworker/app.go)、[阶段 0 汇报](reports/phase-00-foundation.md)。

### 0.4 healthcheck 与 depends_on: service_healthy 分别解决什么问题？

- healthcheck 运行具体探针，区分“容器进程存在”和“依赖真的能响应”。
- `depends_on` 用探针结果安排初始启动顺序，减少应用抢在数据库前启动。
- 探针只证明自己检查过的条件；Kafka 可返回基础元数据与主题可消费不同，应用仍需处理实际就绪条件。

出处：[Compose](../deploy/docker-compose.yml)、[阶段 0 汇报](reports/phase-00-foundation.md)、历史冷启动失败台账 P8-207。

### 0.5 为什么用程序检查包依赖，而不是靠人自觉？

- 隔离规则跨很多文件，代码增长后人工复查容易遗漏。
- depguard 检查默认与 leakbug 构建的导入；archcheck 检查目录布局、标准答案访问和 Compose 挂载，把破坏边界的提交挡在 CI。
- 静态规则能证明特定禁令没有被违反，不能证明业务一定正确，仍需单元、集成和端到端检查。

出处：[depguard 配置](../.golangci.yml)、[archcheck 与样例测试](../tools/archcheck/check_test.go)、[阶段 0 汇报](reports/phase-00-foundation.md)。

## 阶段 1：库存与 MySQL

### 1.1 protobuf 相比 JSON 有什么优缺点？字段编号为什么不能随便改？

- protobuf 用契约定义类型并生成客户端与服务端代码，二进制更紧凑；调试时需要工具，直接可读性弱于 JSON。
- 线上编码使用字段编号识别字段，改编号会让新旧程序对同一数据产生不同理解。
- 删除的编号不能随便复用；不兼容变更通过新契约版本处理，生成文件由 buf 重建。

出处：[库存契约](../api/proto/inventory/v1/inventory.proto)、[阶段 1 汇报](reports/phase-01-inventory.md)。

### 1.2 gRPC deadline 怎么传到服务端，服务端怎么感知？

- 调用方用带截止时间的 context 发起 RPC，gRPC 将剩余预算传播给服务端。
- 服务端从收到的 context 读取取消和截止信息，并把同一个 context 传给业务与数据库。
- 中途换成 Background 会切断传播；真实 deadline 测试证明查询会收到取消，见台账 P1-4。

出处：[gRPC 适配](../internal/shop/inventory/grpc_server.go)、[deadline 集成测试](../internal/shop/inventory/repository_integration_test.go)、[阶段 1 汇报](reports/phase-01-inventory.md)。

### 1.3 为什么条件更新防超卖，而先查后改不行？

- 先查后改有并发空隙：多个请求都能读到最后一件，再分别扣减。
- UPDATE 的 WHERE 同时包含商品身份与余量条件，在数据库锁保护下检查并修改。
- 用影响行数判断是否扣减；实测并发争抢的成功数与最终余量见台账 P1-1，不能只拿 Go 进程内锁代替数据库约束。

出处：[库存 SQL](../internal/shop/inventory/repository.go)、[并发测试](../internal/shop/inventory/repository_integration_test.go)、[阶段 1 汇报](reports/phase-01-inventory.md)。

### 1.4 聚簇索引、二级索引与回表分别是什么？

- InnoDB 按主键组织聚簇索引，叶子保存完整数据行。
- 二级索引叶子保存索引列与主键；查询需要其他列时，再按主键取完整行，叫回表。
- 是否回表、预计扫描多少行、用哪个索引，要对具体 SQL 看 EXPLAIN；本项目删库存索引的实测见台账 P1-2。

出处：[stock 表迁移](../internal/shop/migrations/0002_stock.sql)、[库存仓储](../internal/shop/inventory/repository.go)、[阶段 1 汇报](reports/phase-01-inventory.md)。

### 1.5 没有索引的 UPDATE 为什么会锁住大量行？

- InnoDB 通过索引扫描寻找待更新记录，扫描路径影响加锁范围。
- 没有能缩小扫描的索引时，会读取并锁定更大范围，使本来无关的扣减互相等待。
- 不能只凭“锁等待高”认定缺索引，要同时看计划、索引清单和并发访问分布；删索引对照见台账 P1-2。

出处：[扣减 SQL 讲解](../internal/shop/inventory/repository.go)、[阶段 1 汇报](reports/phase-01-inventory.md)。

### 1.6 批量插入为什么快？

- 一次发送多行，减少网络往返、语句解析和事务提交次数。
- 分批控制内存、SQL 长度和事务大小，避免把所有造数装成一个巨大事务。
- 造数还同步准备库存、销量和已处理消息；实际规模、耗时和重复运行跳过见台账 P1-3。

出处：[造数编排](../internal/shop/seed/seed.go)、[批量 SQL](../internal/shop/seed/repository.go)、[阶段 1 汇报](reports/phase-01-inventory.md)。

### 1.7 MaxOpenConns 设多少，为什么？

- 普通 shop 服务示例最大连接数为 20（代码配置值，见 `.env.example` 的 `DB_MAX_OPEN_CONNS`），可以由环境变量调整；它不是数据库吞吐量。
- 上限过大会把压力转移到 MySQL，过小会让请求在借连接时排队，预算在执行 SQL 前就可能耗完。
- 应结合池利用率、等待数、SQL 耗时与数据库余量调整；医生使用更小的独立池，防止查证抢占业务资源。

出处：[环境示例](../.env.example)、[MySQL 池配置](../internal/platform/mysqlx/mysql.go)、[阶段 1 汇报](reports/phase-01-inventory.md)、[S03 实验说明](learn/phase-04-chaos.md#54-s03连接池只有两条连接)。

## 阶段 2：订单、缓存与消息

### 2.1 Cache Aside 怎样读写？更新时为什么删缓存？

- 读取先查缓存，未命中再查数据库并把结果缓存。
- 更新先保证数据库正确，再使旧缓存失效，让后续读取重新构建。
- 直接改缓存要协调并发更新顺序，旧请求可能覆盖新值；删缓存也不是强一致协议，本项目主要实现商品读取。

出处：[商品服务](../internal/shop/product/service.go)、[商品缓存](../internal/shop/product/cache.go)、[阶段 2 汇报](reports/phase-02-order.md)。

### 2.2 缓存穿透、击穿、雪崩分别怎样应对？

- 穿透是反复查询不存在的数据，短时空值缓存减少重复回源。
- 击穿是热点键失效引起同键并发回源，singleflight 合并同进程查询。
- 雪崩是大量键一起过期，TTL 抖动打散时间；防护对照和有限影响要按实测描述，见台账阶段 4 的 S04/S05 条目。

出处：[缓存实现](../internal/shop/product/cache.go)、[阶段 2 汇报](reports/phase-02-order.md)、[阶段 4 学习材料](learn/phase-04-chaos.md)。

### 2.3 singleflight 原理是什么，能覆盖多个实例吗？

- 同一进程里按 key 记录正在执行的查询，后来的同键请求等待并共享结果。
- 它减少并发未命中时的数据库重复工作，不是缓存本身。
- 多实例各有独立内存和 singleflight，仍可能各回源一次；项目没有用分布式锁把它扩展为全局互斥。

出处：[商品服务](../internal/shop/product/service.go)、[并发缓存测试](../internal/shop/product/cache_test.go)、[阶段 2 汇报](reports/phase-02-order.md)。

### 2.4 Kafka 至少投递一次是什么？为什么数据库先提交？

- 消费重试或位点尚未提交就崩溃，可能再次读到同一消息。
- 若先提交位点再写业务库，崩溃会跳过尚未生效的业务，造成丢处理。
- 先提交数据库再提交位点，把主要风险变成可去重的重复；必须配合事务幂等，而不是宣称恰好一次。

出处：[worker](../internal/shop/app/orderworker/app.go)、[销量事务](../internal/shop/sales/repository.go)、[阶段 2 汇报](reports/phase-02-order.md)。

### 2.5 消费幂等有哪些办法，为什么选事件表？

- 可用唯一键、已处理事件表或业务自身的幂等更新，选择取决于副作用和事务边界。
- 项目在同一 MySQL 事务中插入订单 ID 去重记录并累加销量，唯一键使重复事件可识别。
- 事务原子性避免“记已处理却没加销量”；真实重复消费验证见台账 P2-4。

出处：[销量 SQL](../internal/shop/sales/repository.go)、[重复消费集成](../internal/shop/sales/repository_integration_test.go)、[阶段 2 汇报](reports/phase-02-order.md)。

### 2.6 下单有哪些不一致窗口，Outbox 解决哪个？

- 扣库存成功后订单失败，需要归还库存，但补偿也可能失败或结果未知。
- 订单已提交后发 Kafka 失败，销量可能少一笔；当前保留错误，不假装消息必达。
- Outbox 把业务订单与待发消息写进同一数据库事务，再异步投递，主要解决数据库提交与消息发布窗口；本项目未实现。

出处：[下单流程](../internal/shop/order/service.go)、[阶段 2 汇报的已知限制](reports/phase-02-order.md)、总量核对边界见台账 P2-8。

### 2.7 为什么用 Zipf 而非均匀随机流量？

- 实际访问常有少数热门商品，均匀随机会稀释热点。
- Zipf 让缓存复用、热点键和热点行竞争更容易体现。
- 目标 QPS 不等于实际 QPS，要一起报告 dropped、错误和延迟；真实基准及此前失败轮见台账 P2-1。

出处：[压测生成器](../internal/loadgen/doc.go)、[阶段 2 汇报](reports/phase-02-order.md)。

### 2.8 熔断三种状态是什么，快速失败为什么有用？

- 关闭时正常请求并统计失败；超过规则后打开，直接拒绝调用。
- 打开后等待再半开，只放有限请求试探，成功恢复、失败再打开。
- 快速失败减少故障下游和上游等待资源，商品可以降级；真实停机与恢复见台账 P2-6。

出处：[熔断配置](../internal/platform/resilience/breaker.go)、[库存客户端](../internal/shop/inventoryclient/inventory_client.go)、[阶段 2 汇报](reports/phase-02-order.md)。

### 2.9 GetStock 为什么可重试，Deduct 不行？为什么退避与抖动？

- 只读 GetStock 重复执行不改变库存；Deduct 超时可能已扣减，再发会扣第二次。
- 退避降低连续失败时的请求密度，随机抖动避免大量调用者同时重试。
- 重试还要服从总 deadline、单次预算和次数上限，不能靠重试掩盖失败；S13 展示放大链路。

出处：[重试器](../internal/platform/resilience/retry.go)、[库存客户端](../internal/shop/inventoryclient/inventory_client.go)、[阶段 2 汇报](reports/phase-02-order.md)。

### 2.10 令牌桶与漏桶有什么区别，单机限流有何局限？

- 令牌桶按速率补令牌，允许积攒到容量上限后短时突发。
- 漏桶按固定速度释放请求，整形更平滑，但通常增加排队或拒绝。
- 单机各算各的，实例数变化会改变全局总量；项目入口限流没有实现分布式全局配额。

出处：[HTTP 限流](../internal/platform/resilience/http.go)、[治理测试](../internal/platform/resilience/resilience_test.go)、[阶段 2 汇报](reports/phase-02-order.md)。

### 2.11 Redis 怎样实现接口幂等，记录留多久，故障时为什么拒绝？

- 用 SET NX 和过期时间抢占逻辑请求，完成后保存原响应，同键再次请求回放。
- 保存期要覆盖客户端重试窗口，项目期限由配置明确；过期后不能再保证原逻辑请求去重。
- Redis 不可用时无法确认是否已执行，宁可拒绝，避免重复下单；实测同键回放与 Redis 故障见台账 P2-2、P2-7。

出处：[幂等缓存](../internal/shop/order/cache.go)、[幂等状态](../internal/shop/order/idempotency.go)、[阶段 2 汇报](reports/phase-02-order.md)。

## 阶段 3：可观测性

### 3.1 Counter、Gauge、Histogram 有何区别？

- Counter 只增不减，进程重启归零，如重试次数与已处理消息数。
- Gauge 表示可增可减的当前值，如正在使用的连接数或熔断状态。
- Histogram 按区间累计观察值，如请求耗时，可以聚合桶估计分位数。

出处：[指标注册](../internal/platform/telemetry/metrics.go)、[阶段 3 汇报](reports/phase-03-observability.md)。

### 3.2 P99 怎样从直方图算出，为什么是近似值？

- 先计算各桶增长速率，合并同一桶计数，再找累计比例达到目标的桶。
- 只知道值落在哪个区间，桶内需要插值，因此不是排序原始请求所得的精确值。
- 多实例应合并桶后计算，不能平均各实例或各时间窗的 P99。

出处：[观测指标](../internal/platform/telemetry/metrics.go)、[看板](../deploy/grafana/dashboards/shop.json)、[阶段 3 汇报](reports/phase-03-observability.md)。

### 3.3 trace、span、context 是什么，怎样跨服务和 Kafka？

- trace 是一次请求的完整路径，span 是其中一步，context 携带当前 span 和取消等状态。
- HTTP/gRPC 在载体里传播 traceparent，下游提取后建立子 span。
- Kafka 手动向消息头注入和提取；实际跨服务链路见台账 P3-11，不能只看单个服务的 span。

出处：[Kafka 传播](../internal/platform/telemetry/kafka.go)、[传播测试](../internal/platform/telemetry/tracing_test.go)、[阶段 3 汇报](reports/phase-03-observability.md)。

### 3.4 为什么告警基于症状，不基于原因？

- 用户可见的延迟、错误与积压能够覆盖多种根因。
- 把“缺索引”写进告警会把尚未查证的推测当事实，也泄露实验答案。
- 医生应结合计划、锁、池和调用链建立因果，不能把告警名称当最终结论。

出处：[告警规则](../deploy/prometheus/rules/README.md)、[阶段 3 汇报](reports/phase-03-observability.md)。

### 3.5 指标标签为什么不能用用户 ID？

- 每种标签组合形成新时序，用户、订单和 trace ID 会让组合数持续膨胀。
- 高基数增加采集、内存、存储和查询压力，可能使监控本身不稳定。
- 指标保留低基数的路由模板、方法和结果；具体请求身份放日志与 trace。

出处：[指标中间件](../internal/platform/telemetry/middleware.go)、[阶段 3 汇报](reports/phase-03-observability.md)。

## 阶段 4：故障注入

### 4.1 S01 与 S10 都是 SQL 慢，靠什么区分？

- S01 看缺失索引、全表扫描和扫描扩大后的锁影响，计划变化是直接线索。
- S10 索引和计划正常，要比较客户端 SQL span 与数据库内部执行，判断额外时间是否在往返链路。
- 连接池排队是结果也可能是根因，不能见到排队就停止；正式 S10 带工具的 12 份均误归连接池，台账 P8-149、P8-153、P8-157。

出处：[S01/S10 原理](learn/phase-04-chaos.md)、[阶段 4 汇报](reports/phase-04-chaos.md)、[阶段 8 失败分析](reports/phase-08-failure-analysis.md)。

### 4.2 连接池太小为什么使 gRPC 超时？

- 请求的 deadline 从调用开始计算，借连接的等待也占预算。
- 全部连接被占用时，新 SQL 在进入数据库前排队，可能尚未执行就超时。
- 要一起看池等待、连接占用、SQL 内部执行与锁等待，区分池配置不足和慢 SQL 拉长占用。

出处：[MySQL 池](../internal/platform/mysqlx/mysql.go)、[S03 原理](learn/phase-04-chaos.md)、[阶段 4 汇报](reports/phase-04-chaos.md)。

### 4.3 S04 为什么发布关闭 singleflight 的配置？

- 正常保护会把同键并发未命中合并，难以观察重复回源。
- 关闭它模拟一次撤掉保护的真实配置发布，业务查询本身不加假延迟。
- 商品读取突发与下单热点要分开，否则会把行锁竞争误当缓存击穿；真实返工见阶段 4 记录。

出处：[商品 singleflight](../internal/shop/product/service.go)、[阶段 4 汇报](reports/phase-04-chaos.md)、[S04 开发回放](learn/phase-04-chaos.md)。

### 4.4 S08 热点行锁与 S01 缺索引锁等待有什么不同？

- S08 有正常索引，许多事务准确定位并争抢同一热点库存行。
- S01 缺索引，扫描与加锁范围扩大，不止正常热点竞争。
- 二者都能表现为等待，必须结合访问分布、计划和索引，而不是只看锁指标。

出处：[库存 SQL](../internal/shop/inventory/repository.go)、[场景原理](learn/phase-04-chaos.md)、[阶段 4 汇报](reports/phase-04-chaos.md)。

### 4.5 为什么注入原因，而不伪造症状？

- 真实原因自然影响多个观测维度，能验证证据之间是否相互支持。
- 人工返回假错误只证明程序识别了预设输出，不能检验排障推理。
- 修改索引、网络、流量等动作有可核对的恢复方式，实验要同时验证症状与恢复健康。

出处：[注入器分层](../internal/chaos/README.md)、[阶段 4 汇报](reports/phase-04-chaos.md)。

### 4.6 为什么用发布配置或版本制造故障，而不用故障开关？

- 错误发布是现实事故来源，启动日志、版本指标和重启是正常可见线索。
- 特殊开关、函数名或故障日志会向医生直接泄露答案。
- 缺陷构建用独立标签和镜像，恢复必须核对原镜像内容，不能随意用浮动标签。

出处：[发布动作](../internal/chaos/action/doc.go)、[Compose](../deploy/docker-compose.yml)、[阶段 4 汇报](reports/phase-04-chaos.md)。

### 4.7 S13 重试风暴怎样形成，熔断与退避怎样作用？

- 下游延迟使单次尝试超时，若总预算尚有余量且立即重试，入口流量被放大。
- 退避和抖动降低连续重试密度，熔断在失败持续时暂停向下游发送。
- 次数上限不等于一定执行该次数，仍受总预算与取消约束；扣减始终不能自动重试。

出处：[重试实现](../internal/platform/resilience/retry.go)、[单次预算测试](../internal/shop/inventoryclient/attempt_test.go)、[阶段 4 汇报](reports/phase-04-chaos.md)。

## 阶段 5：数据源与只读工具

### 5.1 为什么接口定义在使用方，和 Java 有什么不同？

- 使用方知道所需的最小能力，接口不必跟着具体客户端所有方法膨胀。
- Go 按方法集合隐式满足接口，不需要实现方显式声明 implements。
- Java 同样能按需求设计小接口；差异在实现声明与包组织习惯，不能说 Java 做不到。

出处：[source 接口](../internal/doctor/source/source.go)、[阶段 5 汇报](reports/phase-05-doctor-tools.md)。

### 5.2 适配器模式解决什么问题？

- 上层问“搜索某服务日志”，不应知道 Loki 的返回外壳和 LogQL 转义细节。
- 适配器把统一请求翻译为后端协议，再整理为统一结果，便于假后端测试。
- 日志已实现 Loki 和 OpenSearch 两个适配器，后者已查询官方 Demo 的真实日志，见台账 P9-3；陌生系统的诊断效果还要由阶段 9 独立结果证明。

出处：[日志接口](../internal/doctor/source/source.go)、[Loki 实现](../internal/doctor/source/loki/client.go)、[阶段 5 汇报](reports/phase-05-doctor-tools.md)。

### 5.3 工具结果为什么截断与摘要？

- 大结果会占据模型上下文、增加响应耗时与费用，也可能让故障数据库负担更重。
- 摘要保留结构与重点，统一截断提供硬边界，并标明结果不完整。
- 取舍是可能丢失关键证据，模型应缩小查询继续取证，不能把截断当成完整原始数据。

出处：[注册表](../internal/doctor/tools/registry.go)、[摘要](../internal/doctor/tools/summary.go)、[阶段 5 汇报](reports/phase-05-doctor-tools.md)。

### 5.4 为什么只读账号是最后一道防线？

- 即使工具校验出错，数据库仍独立拒绝业务写入，Redis ACL 拒绝内容读取与危险命令。
- 只读账号不是无风险：MySQL 为 EXPLAIN 授予 SELECT，工具层仍要保证不返回数据行。
- 权限需要真实后端验证，不能只用 Mock 证明数据库拒绝写入。

出处：[MySQL 权限集成](../internal/doctor/source/mysqlinspect/permissions_integration_test.go)、[Redis 权限集成](../internal/doctor/source/redisinspect/permissions_integration_test.go)、[阶段 5 汇报](reports/phase-05-doctor-tools.md)。

### 5.5 mysql_explain 为什么只接受 SELECT？

- 缩小 SQL 输入范围，并由程序自己加 EXPLAIN 前缀。
- 拒绝多语句、已有 EXPLAIN 和 ANALYZE，避免 EXPLAIN ANALYZE 真正执行查询。
- 分析扣减 UPDATE 时改写为条件相同的 SELECT；计划用于证据，不返回业务行。

出处：[SQL 校验](../internal/doctor/source/select.go)、[校验测试](../internal/doctor/source/select_test.go)、[阶段 5 汇报](reports/phase-05-doctor-tools.md)。

### 5.6 告警为什么去重？

- Alertmanager 会正常重复通知，网络失败也会重发，同一事故可能持续多条症状。
- 若每条都建诊断，会产生重复查证、费用和互相冲突的写建议。
- 事务归并系统与故障组，报告后仍接收后续通知；恢复观察期结束后才允许新自动事故。

出处：[告警事务](../internal/doctor/store/alerts.go)、[故障开放期](../internal/doctor/store/faults.go)、[阶段 5 汇报](reports/phase-05-doctor-tools.md)。

### 5.7 删掉系统提示的安全规则，医生能多看什么？

- 模型意图可能变化，但不会新增工具、挂载、固定地址或账号权限。
- 参数校验、审批门禁、账号与资源上限仍由代码执行。
- 提示是辅助约束而非权限边界；当前实际抗攻击暴露实验没有完成，不能把代码边界扩大成模型永远不受骗。

出处：[工具校验](../internal/doctor/tools/catalog_test.go)、[审批测试](../internal/doctor/agent/approval_test.go)、[阶段 5 汇报](reports/phase-05-doctor-tools.md)、S12 限制台账 P8-204。

### 5.8 多系统如何隔离，为什么参数里不放系统名？

- 创建工具集时就绑定本系统的数据源、账号和地址，诊断创建后系统不可变。
- 工具调用参数不允许临时切换系统，也不能提供新 URL。
- 配置重复与共享凭据拒绝启动，告警令牌和 MCP 令牌各按系统绑定；测试中的第二系统不是已完成真实泛化。

出处：[目标目录](../internal/doctor/target/catalog.go)、[隔离测试](../internal/doctor/target/catalog_test.go)、[阶段 5 汇报](reports/phase-05-doctor-tools.md)。

## 阶段 6：Agent 与页面

### 6.1 Tool Calling 一轮里模型返回什么，程序回什么，tool_call_id 有何用？

- 模型提出工具名、JSON 参数与调用 ID，不直接执行函数。
- 程序校验、调用固定工具、保存步骤，再回传结果及相同调用 ID。
- ID 关联并行调用与结果；项目再结合模型步骤号，避免供应商跨轮复用 ID 时混淆。

出处：[模型类型](../internal/doctor/llm/model.go)、[工具往返](../internal/doctor/agent/calls.go)、[阶段 6 汇报](reports/phase-06-agent.md)。

### 6.2 Agent 何时结束，为什么必须有预算？

- 合法提交报告、用户取消、不可继续的错误或预算耗尽都会使任务结束。
- 时间、轮数、工具、上下文、累计 token 与费用限制不同资源，不能只设其中一个。
- 查证预算用尽后留空间整理报告与有限修正，防止无穷查询、猜测指标名或无限花费。

出处：[循环](../internal/doctor/agent/loop.go)、[预算](../internal/doctor/agent/budget.go)、[阶段 6 汇报](reports/phase-06-agent.md)。

### 6.3 errgroup 怎样用，一个工具失败会取消其他工具吗？

- 项目用 `errgroup.Group` 加并行上限等待同轮只读查询，不使用“普通工具错误触发整组取消”的策略。
- 参数、超时或后端错误包装成工具结果，其他查询仍可能提供有效证据；审计失败才作为组错误返回。
- 父 context 取消仍传给所有查询；每个任务写独立切片元素，Wait 后才合并历史，避免共享 append 竞态。

出处：[readTools](../internal/doctor/agent/calls.go)、[并行与取消测试](../internal/doctor/agent/loop_test.go)、[阶段 6 汇报](reports/phase-06-agent.md)。

### 6.4 点击取消怎样让模型 HTTP 请求停下来？

- HTTP 取消入口通知调度器，调度器取消该任务的 context。
- 同一个取消链传入 Agent、工具和模型的 `NewRequestWithContext`，底层 I/O 因取消返回。
- 独立收尾 context 只保存结束状态，不继续生成或查证；取消测试还要确认在途任务退出。

出处：[取消接口](../internal/doctor/httpapi/diagnostics.go)、[调度器](../internal/doctor/run/scheduler.go)、[阶段 6 汇报](reports/phase-06-agent.md)。

### 6.5 SSE 与 WebSocket 如何取舍，怎样断线续传？

- 页面主要接收服务器步骤，单向 SSE 比双向协议更直接。
- 每条事件有持久化步骤编号，重连携带 Last-Event-ID，先从数据库补发缺失编号。
- 内存通知只是唤醒，慢客户端可以断开再补；不能让网页速度拖住 Agent。

出处：[SSE 实现](../internal/doctor/httpapi/stream.go)、[断线补发测试](../internal/doctor/httpapi/stream_test.go)、[阶段 6 汇报](reports/phase-06-agent.md)。

### 6.6 提示词为什么需要版本号？

- 同一模型在不同提示下可能改变工具顺序、证据选择和报告。
- 版本与原始审计使失败可以追溯，避免把不同条件混进同一成绩。
- 正式阶段 8 冻结提示、工具、手册和标准；阶段 10 只整理展示，不能改完继续沿用旧分数。

出处：[提示嵌入与版本](../internal/doctor/agent/prompt.go)、[阶段 6 汇报](reports/phase-06-agent.md)、冻结台账 P8-38、P8-194。

## 阶段 7：审批与可靠性

### 7.1 为什么明确写状态转换表？

- 状态决定任务是否可执行、可取消、可审批或可恢复。
- 集中转换规则避免各 HTTP 接口和后台任务各写一套含糊判断。
- 非法转换直接拒绝并测试，终态不能被重新执行覆盖。

出处：[状态机](../internal/doctor/run/state.go)、[调度测试](../internal/doctor/run/scheduler_test.go)、[阶段 7 汇报](reports/phase-07-reliability.md)。

### 7.2 租约与锁有什么区别，为什么用租约？

- 进程内锁只在当前进程有效，崩溃后无法说明数据库任务归谁执行。
- 租约在数据库保存拥有者与到期时间，执行者续约；到期后才允许接手。
- 写步骤时还要校验拥有者，续租失效就停止，防止旧执行者恢复后覆盖新执行者。

出处：[领取与续租](../internal/doctor/store/leases.go)、[租约集成测试](../internal/doctor/store/lease_integration_test.go)、[阶段 7 汇报](reports/phase-07-reliability.md)。

### 7.3 为什么恢复可补只读工具，却不能直接重放写操作？

- 已有只读结果原样回放，缺失结果可补查，但新结果属于新时刻，因此还要限制断点过时。
- 写操作超时或进程崩溃时，远端可能已经成功，重发会产生第二次副作用。
- 先核对索引形状、实例修订号或缓存状态，再确认或继续；真实杀进程恢复见台账 P7-14。

出处：[对话恢复](../internal/doctor/agent/replay.go)、[执行恢复](../internal/doctor/approval/execute.go)、[阶段 7 汇报](reports/phase-07-reliability.md)。

### 7.4 审批幂等怎样用 SQL 保证？

- 用条件 UPDATE 要求当前仍 pending 且未过期，再改成批准或拒绝。
- 并发决定只有一个影响行数成功，后来的请求看到状态已变就不能再决定。
- 执行还要独立领取与核对，不能把“批准一次”直接等同于任意远端副作用恰好一次。

出处：[审批 SQL](../internal/doctor/store/approvals.go)、[争抢测试](../internal/doctor/store/approval_integration_test.go)、[阶段 7 汇报](reports/phase-07-reliability.md)。

### 7.5 什么是提示注入，为什么不能只让 AI 别上当？

- 日志、手册等外部文字可能伪装成应遵从的指令，诱导模型偏离任务。
- JSON 字符串包装和系统提示辅助区分数据与指令，权限由工具、审批、账号和资源层执行。
- S12 攻击文字未被医生实际读到，防注入目前只有权限、审批与隔离测试作证据，不能宣称实际暴露后防御成功。

出处：[不可信工具正文](../internal/doctor/agent/calls.go)、[注入与回放测试](../internal/doctor/agent/replay_test.go)、[阶段 7 汇报](reports/phase-07-reliability.md)、台账 P8-204。

### 7.6 MCP 和 Tool Calling 什么关系？

- Tool Calling 是模型表达“调用哪个函数、传什么参数”的结构化方式。
- MCP 是外部客户端发现与调用工具的协议，不自动赋予模型执行权。
- 本项目两条入口复用只读注册表；MCP 按系统令牌隔离并审计，没有写工具与审批接口，真实接线见台账 P7-8。

出处：[MCP 服务](../internal/doctor/mcpserver/doc.go)、[接入说明](mcp.md)、[阶段 7 汇报](reports/phase-07-reliability.md)。

### 7.7 执行为什么只用审批单保存的参数？

- 人批准的是具体语句与参数，不是模型一句抽象建议。
- 若执行时再让模型传参，会出现审阅内容与实际执行内容不同的风险。
- 保存的计划是执行依据；参数改变必须重新申请并让人看到风险与撤销方法。

出处：[审批服务](../internal/doctor/approval/service.go)、[原单参数测试](../internal/doctor/approval/service_test.go)、[阶段 7 汇报](reports/phase-07-reliability.md)。

### 7.8 模型完全受控制，最坏能做到什么？

- 在预算内读取当前系统允许的证据，并申请建索引、调池或预热缓存。
- 模型没有审批入口、任意 URL、shell 或 Docker；固定客户端与账号挡住跨系统和任意业务写入。
- 人误批仍可能让数据库耗资源或服务变慢；任何登录用户都可审批、脱敏不完整，生产还需角色隔离和数据治理。

出处：[写参数范围](../internal/doctor/approval/prepare.go)、[真实权限测试](../internal/doctor/approval/permissions_integration_test.go)、[阶段 7 汇报](reports/phase-07-reliability.md)。

## 阶段 8：评测

### 8.1 为什么每个场景多跑几次？

- 模型回答与运行环境都有波动，单次成功可能偶然。
- 正式自建靶场每个场景 3 次，见台账 P8-191；这个样本量仍小，应同时看均值、范围和失败。
- 它不是生产准确率，也不能替代陌生系统测试；阶段 9 的外部日志接入已经实测，诊断效果已在外部小样本独立实测（P9-77），不能推广为生产准确率。

出处：[自动评测](../internal/eval/runner/automatic.go)、[阶段 8 汇报](reports/phase-08-eval.md)、[正式原结果](reports/phase-08-formal-results.md)。

### 8.2 大模型评委有什么风险，校准解决什么，本项目做了什么？

- 评委会漏看证据、偏好表达、串错因果，合法 JSON 也可能给错分。
- 人工校准本可用人的独立判断检查尺度；本次未做人工校准，用户改用 Claude 独立盲评，两个模型可能有共同偏差。
- DeepSeek 评委与 Claude 交叉评分的一致率：全量机制 17/20（85%），去掉 E0 为 12/15（80%），台账 P8-199、P8-201；不能叫评委与人的一致率。
- 证据判定 DeepSeek 比 Claude 多给 11 项、少给 4 项，台账 P8-202；保留分歧而不把 Claude 当绝对答案。

出处：[评分协议](../internal/eval/judge/protocol.go)、[交叉评分对照](reports/phase-08-cross-scoring.md)、[阶段 8 汇报](reports/phase-08-eval.md)。

### 8.3 什么是过拟合，项目怎样减少它？

- 过拟合是只适应熟悉题目的线索，换系统就失效。
- 医生不能读标准答案；说明书只描述架构，手册写通用方法，正式冻结后不按失败场景改提示刷分。
- 这些只能降低风险，尚不能证明诊断泛化；阶段9已独立实测，仍不能把接口抽象或日志接入成功等同于诊断效果证据。

出处：[架构检查](../tools/archcheck/main.go)、[冻结校验](../internal/eval/runner/manifest.go)、[阶段 8 汇报](reports/phase-08-eval.md)。

### 8.4 E3 消融说明了什么？

- E3 去掉调用链工具，尽量只改变这一能力，再与同故障的 E1 比较。
- 要同时看组件、机制、证据覆盖与具体失败，不能只看最终报告是否生成。
- 日志、指标也可能提供相关线索，评委还可能把堆栈当调用链证据；这只能说明当前冻结实验中的贡献，不能推出所有系统结论。

出处：[执行配置](../internal/doctor/run/execution.go)、[阶段 8 汇报](reports/phase-08-eval.md)、[失败分析](reports/phase-08-failure-analysis.md)，评分疑点台账 P8-56、P8-60。

### 8.5 怎样区分没拿到证据和查到了却推理错？

- 先按保存步骤看工具真实返回，不从报告自称“已查”推断。
- 关键事实没出现时，再区分工具不支持、后端没数据、范围不对或查询被拒。
- 事实已出现却忽略、读反或混淆上下游因果，才归入推理问题；S10 的连接池误判就是不能把现象当根因的例子。

出处：[证据评分](../internal/eval/scoring/evidence.go)、[逐场失败分析](reports/phase-08-failure-analysis.md)、[阶段 8 汇报](reports/phase-08-eval.md)，台账 P8-149、P8-153、P8-157。

## 阶段 9：陌生系统与第二日志后端

### 9.1 在开源靶场上的准确率比自己的低多少，主要原因是什么？

- 同一Terra模型，外部正式E1/E2各五份，组件严格一致均3/5（60%）；自建shop对应两组各36/39。阶段8为十三场各三次，阶段9为另一系统五场各一次，分母与故障类型分别列，不合并统计（P10-2、P10-3、P9-77）。
- 外部两组机制均为正确2、部分2、错误1，coverage均值75%，各五份原评分有效；不是每份都查明根因。OT02输出`cart->valkey-cart`与GT`cart`不严格相等；OT04保留unknown，OT05缺具体商品和关键因果（P9-77）。
- 分数差包含严格命名、缺失证据和推理范围，不宜只归因为陌生架构或工具变少。没有为改善成绩改提示、手册、工具或评分，保留原报告和原DeepSeek理由；评委也可能有尺度偏差。
- pilot2的评分失败与Mock占位结果均不纳入正式成绩，评分失败不能按零分计入机制/覆盖分母。

出处：[新正式结果](reports/phase-09-formal-results.md)、[原件/独立统计](reports/phase-09-formal-snapshot.json)、[自建结果](reports/phase-08-formal-results.md)、[台账](数据台账.md) P9-77、P10-2、P10-3。

### 9.2 接入陌生系统，最花时间的是哪一步？

- 没有单独计量完整净接入工时，所以不声称哪项耗时最长或“几分钟接完”。已实测部署、登录菜单和真实只读查询等分段时间；最近缓存镜像启动Demo为41.2169576秒，不是开发或全新下载耗时（P9-25–P9-29、P9-70）。
- 实际卡点依次包括多Compose文件/固定镜像、Node健康探测、OpenSearch索引及字段映射冲突、Jaeger base path、Collector起点审计、稀疏业务导致单次基线失败。原失败与补丁范围都保留。
- 日志接通要核真实index/mapping及五字段，查询成功还要查采集丢弃；接口抽象解决翻译问题，不自动解决采集质量或数据缺失。

出处：[阶段9报告](reports/phase-09-generalization.md)、[采集专项](reports/phase-09-logging-fix.md)、[原生采样](reports/phase-09-native-metrics.md)、[接入指南](onboarding.md)，台账P9-13–P9-16、P9-25–P9-29、P9-70。

### 9.3 数据缺失时，医生的表现发生了什么变化？

- otel-demo只注册8项只读工具，shop为16项；PostgreSQL检查、Valkey直查与pprof缺失时不注册相应工具。指标中有数据库统计不等于能查执行计划和锁，缺数据不能借用shop账号或临时万能工具绕过（P9-8）。
- 外部正式结果里OT04两组确认结账异常但根因unknown；OT05定位到product-catalog后仍缺具体商品证据，机制仅部分正确。能说报告在具体缺证据处保留未知，不能凭两套不同故障集合把全部差值都归因于降级（P9-77）。
- 这种降级是如实缩小能力范围，不承诺每个已注册工具都有有效预置指标；例如kafka_consumer_lag、recent_changes可能查不到本系统所需指标。

出处：[第二系统配置](../deploy/doctor/targets/otel-demo.yaml)、[装配](../internal/doctor/target/catalog.go)、[正式逐报告](reports/phase-09-formal-results.md)，台账P9-8、P9-77。

### 9.4 同一次 search_logs 调用，两个后端收到什么，为什么不让 AI 写查询？

- 调用者只给服务、级别、关键词、trace、时间和条数。Loki 收到固定形状的 LogQL；时间与上限放到 query_range 参数。
- OpenSearch 收到固定索引 `_search` 的 JSON：服务和 trace 用精确 term，级别的完整值忽略大小写，时间用 range，正文关键词用 match_phrase，并按时间倒序。
- 字符串交给转义或 JSON 编码器，只作为值。AI 不获得任意索引、字段、后端地址和查询结构的选择权，边界由代码兑现。
- 两种产品的文本分析语义仍有细节差异；共用接口不等于任意中文、标点或短语都逐字产生相同命中。

出处：[Loki 翻译](../internal/doctor/source/loki/client.go)、[OpenSearch 翻译](../internal/doctor/source/opensearch/client.go)、[并排解释](learn/phase-09-generalization.md#51-同一个接口的第二个实现)。

### 9.5 为什么字段名放进接入配置？

- 同一份日志经不同采集链路后，服务和时间字段的位置、名称可能不同；这些属于部署信息，不应为每套系统重写适配器。
- 先用真实 mapping 与样本确认，再校验五项映射。精确匹配常用的 `.keyword` 子字段不在原始 `_source` 中，响应解析还要兼容带点号的完整键。
- 同时配置 Loki、OpenSearch 会报错，避免由装配顺序静默决定查哪一个后端。

出处：[字段配置](../internal/doctor/source/opensearch/config.go)、[响应解析](../internal/doctor/source/opensearch/response.go)、[接入校验测试](../internal/doctor/target/opensearch_test.go)、台账 P9-3。

### 9.6 契约测试与普通单元测试有什么不同，这里证明了什么？

- 契约测试也可以是单元级测试，区别在关注点：它用同一张行为表检查不同实现是否兑现同一接口承诺。
- 这里 Loki 与 OpenSearch 共用查询条件和期望日志，同时分别验证发出的后端请求，覆盖服务、级别、关键词、trace、时间、排序、数量、空结果与取消。
- 产品自己的单元测试再检查部分分片失败、重定向、字段缺失和响应边界。真实后端连通还要集成测试；不能由本地模拟测试推断真实集群性能、账号权限或泛化准确率。

出处：[共同契约表](../internal/doctor/source/log_contract_test.go)、[OpenSearch 边界测试](../internal/doctor/source/opensearch/client_test.go)、[真实集成测试](../internal/doctor/source/opensearch/integration_test.go)、台账 P9-3。

### 9.7 为什么先前暂停，后来怎样安全继续？

- 原pilot2医生主动提交、采集normal，但评委JSON拒收；机制与覆盖缺失，不用默认0补分，不重评，原费用与interrupted状态保留（P9-51–P9-55）。
- 用户撤回“评委必须成功才继续”：本新run前两份已返回且计量可确认的冻结校验拒收可继续，累计第三份停；API有限重试后失败、返回或费用未知、余额预算不足以及其他执行/采集/时限异常仍停（P9-58）。本次正式十份评分都有效，没有借新规则重评旧样本（P9-77）。
- 历史0/0点仍判不健康、原回放失败保留。后来只改基线等待：五分钟内每五秒查一轮，必须同轮全部场景健康；开关/开放期检查和原两分钟谓词不变，查询错立即停（P9-67）。
- 原生数据核实业务span样本中位59.997秒与计数平台；Demo下单稀疏，单次检查容易遇到空增量。Mock曾等待35.0503469秒、八轮后同轮全过，五场症状/恢复均在五分钟内（P9-71–P9-73）。这不是放宽健康条件，更不能把increase外推数当订单数。
- 先新代码CI，再真实双Mock完整注入/评分/恢复/导出，最后88项原件和预算/余额核对全过，按原授权直接开新正式run，不再试跑。正式费用USD$0.325033、CNY¥0.196047分列，美元按项目用量账本计，Key共用、平台账单无法单独核对（P9-69、P9-75、P9-78）。

出处：[基线限定审计](reports/phase-09-baseline-wait-amendment.json)、[双Mock计时](reports/phase-09-dualmock-timing.md)、[放行原件](reports/phase-09-dualmock-entry-gate.json)、[新正式结果](reports/phase-09-formal-results.md)、[最终资源](reports/phase-09-final-runtime.json)。

## 阶段 10：整体展示

### 10.1 用三分钟介绍这个项目

- 先说问题：排障要在指标、链路、日志和数据库之间拼证据；项目让受限 Agent 完成查证并生成带来源的报告。
- 接着讲结构：Go 靶场、观测栈、医生、真实注入和独立评测；Mock 演示流程，真实效果引用冻结档案。
- 用一项具体能力与一项失败说明工程可信度：条件更新防超卖见台账 P1-1；S10 误判见 P8-149、P8-153、P8-157。
- 最后明确边界：写操作人工审批、外部小样本不代表生产泛化、交叉评分非人工校准、S12 未暴露。

出处：[README](../README.md)、[架构](architecture.md)、[阶段 8 汇报](reports/phase-08-eval.md)。脚本实测见台账 P10-7：首次走查 189.890 秒、再次排练 152.167 秒，原始 [计时与接口证据](reports/phase-10-fresh-demo.json) 一起保留，不把首次较慢记录删掉。

### 10.2 用十五分钟讲清架构和关键取舍

- 沿下单请求讲接口、事务、缓存、消息和一致性窗口，再展示怎样观测它。
- 沿告警到报告讲目标绑定、持久化、调度、Tool Calling、报告引用与 SSE。
- 沿写申请讲审批、有限账号、恢复核对与最坏情况，说明为什么不用任意 SQL 或命令工具。
- 沿注入到评测讲答案隔离、配对顺序、冻结、评分失败与费用，最后解释单实例和阶段 9 的范围取舍。

出处：[架构两条流程](architecture.md)、[医生组装](../internal/doctor/app/doc.go)、[阶段 7 汇报](reports/phase-07-reliability.md)、[阶段 8 汇报](reports/phase-08-eval.md)。

### 10.3 项目局限是什么，生产还缺什么？

- 已归档的真实诊断效果以自建 shop 的冻结实验为依据；日志已支持 Loki 与 OpenSearch，后者通过官方 Demo 的真实查询，见台账 P9-3。陌生系统正式结果已独立归档（P9-77），仍不支持 PostgreSQL 直查、SkyWalking 与非 Go 运行时分析。
- 评委与医生都可能错，评分失败和尺度分歧保留；没有人工校准，也没有真实暴露后的 S12 抵抗证据。
- 医生单实例、没有角色审批隔离与完整个人信息脱敏，生产还需部署安全、审计保留策略和更广故障验证。
- 业务没有 Outbox，补偿可能失败；总量一致不能代替逐笔一致或可靠消息保证。

出处：[架构限制](architecture.md)、[下单流程](../internal/shop/order/service.go)、[阶段 8 汇报](reports/phase-08-eval.md)，台账 P2-8、P8-191、P8-202、P8-204。

### 10.4 企业不会给 AI 这么大权限，怎么回答？

- 先拿出“能看、能改、不能碰”的具体边界，让对方确认需求，而不是要求信任模型。
- 代码只提供固定只读查询，写操作保存具体计划并由人批准；数据库、Redis 和运维账号进一步限制范围。
- 模型不能自行批准、访问任意 URL、执行 shell 或操作 Docker，审批结果也有审计和撤销方法。
- 坦诚说明 SELECT 与 INDEX 的底层权限、误批影响、任何登录用户可审批和脱敏限制，真实接入前要收紧组织权限。

出处：[README 权限表](../README.md)、[账号权限测试](../internal/doctor/source/mysqlinspect/permissions_integration_test.go)、[阶段 7 汇报](reports/phase-07-reliability.md)。

## 学习材料补充追问

以下补齐阶段学习材料增加的必答题，编号中的“补”表示不属于原计划书的问题。

### 0.补1 sql.DB 与 sql.Conn 有什么区别，命名锁为什么用同一连接？

- `sql.DB` 是池，连续调用可能借到不同连接；`sql.Conn` 固定一条会话。
- MySQL 命名锁属于会话，拿锁、迁移与释放必须保持同一连接。
- 解锁不确定时丢弃连接，不能把仍可能持锁的会话还给池。

出处：[迁移实现](../internal/platform/migrate/migrate.go)、[阶段 0 汇报](reports/phase-00-foundation.md)。

### 0.补2 loc=UTC 与 MySQL time_zone 为什么都设？

- `loc` 决定驱动怎样把日期解释为 Go 的 time.Time。
- 会话 `time_zone` 影响数据库时间函数和时间类型转换。
- 只设一侧会产生看似都叫时间、实际口径不同的问题，因此新连接统一设置。

出处：[连接配置](../internal/platform/mysqlx/mysql.go)、[阶段 0 汇报](reports/phase-00-foundation.md)。

### 0.补3 为什么迁移不能整批包一个事务？

- MySQL DDL 会隐式提交，不能按普通业务事务期待整体回滚。
- 迁移按编号和校验和记录，一文件一语句，失败时保留准确位置。
- 已提交迁移不修改，通过新增迁移演进，避免不同环境版本漂移。

出处：[迁移执行](../internal/platform/migrate/migrate.go)、[迁移文件校验](../internal/platform/migrate/files.go)、[阶段 0 汇报](reports/phase-00-foundation.md)。

### 0.补4 限制请求体为什么不能只看 Content-Length？

- 客户端可能没有提供长度，也可能通过分块传输发送正文。
- 真正读取时限制字节数，才能覆盖没有可信长度的情况。
- 超限返回明确错误，避免解析巨大正文占满内存。

出处：[HTTP 中间件](../internal/platform/httpx/httpx.go)、[阶段 0 汇报](reports/phase-00-foundation.md)。

### 0.补5 迁移并发测试的结果 channel 为什么要有缓冲？

- 测试让并行调用分别返回结果，主流程还需要控制启动与等待顺序。
- 缓冲覆盖同时返回的发送者，防止测试编排先等待结束、发送者却等接收形成死锁。
- 容量来自实际并发数，不是越大越好；需同时确认发送、接收和关闭责任。

出处：[迁移并发测试](../internal/platform/migrate/migrate_integration_test.go)、[阶段 0 汇报](reports/phase-00-foundation.md)。

### 0.补6 怎样确认单元测试不连库，怎样打开集成？

- 业务用假接口，适配器用本地模拟后端，模型用 Mock，清除真实凭据。
- 集成只有显式 TEST 变量才运行，并校验专用库或专用 Redis DB。
- Skip 不是集成通过，要单独运行集成并核对零跳过，检查范围记录见台账 P8-206。

出处：[CI](../.github/workflows/ci.yml)、[库存集成保护](../internal/shop/inventory/repository_integration_test.go)、[阶段 0 汇报](reports/phase-00-foundation.md)。

### 0.补7 commentcheck、depguard、archcheck、race 分别证明什么？

- commentcheck 检查说明存在与文件清单，不证明注释讲得清楚。
- depguard 检查当前构建的导入禁令；archcheck 检查目录布局、标准答案访问与 Compose 边界，两者都不证明远端账号真的受限。
- race 检测测试实际执行路径中的并发数据竞争，不证明所有可能调度都安全。

出处：[commentcheck](../tools/commentcheck/main.go)、[depguard 配置](../.golangci.yml)、[archcheck](../tools/archcheck/main.go)、[阶段 0 汇报](reports/phase-00-foundation.md)。

### 1.补1 扣减失败为什么先看影响行数，再查商品存在？

- 正常成功路径只做条件更新，避免每次先查一次。
- 零影响可能是库存不够，也可能商品不存在，需要补查区分错误。
- 成功路径与错误分类分开，仍保持原子扣减的并发边界。

出处：[库存仓储](../internal/shop/inventory/repository.go)、[阶段 1 汇报](reports/phase-01-inventory.md)。

### 1.补2 为什么 GracefulStop 还要配 Stop？

- GracefulStop 等在途 RPC 完成，可以保护正常请求。
- 卡住的 RPC 可能一直不结束，所以外层还要有停机预算。
- 预算到期调用 Stop 强制退出，保证进程能结束，并承认在途请求可能失败。

出处：[gRPC 停机](../internal/platform/grpcx/shutdown.go)、[阶段 1 汇报](reports/phase-01-inventory.md)。

### 1.补3 造数为什么写销量和已处理记录？

- 初始订单不能让销量基线一开始就不相等。
- 已处理事件记录说明造数订单的销量已经入账，避免以后同订单消息再加一次。
- 这些写入按事务和批次保持一致，重复 seed 检测已有数据并跳过，实测见台账 P1-3。

出处：[造数仓储](../internal/shop/seed/repository.go)、[阶段 1 汇报](reports/phase-01-inventory.md)。

### 2.补1 IN_PROGRESS 与 OUTCOME_UNKNOWN 差在哪？

- 前者表示原请求仍在处理中，暂时不能并发重复执行。
- 后者表示副作用可能已经发生，需要核对结果，不能换键重新下单。
- 只有确认无残留副作用的失败才删除占位，让同键安全重试。

出处：[幂等状态](../internal/shop/order/idempotency.go)、[阶段 2 汇报](reports/phase-02-order.md)。

### 2.补2 修改幂等记录为什么比较整条记录？

- 处理超时后旧请求可能比新持有者更晚返回。
- 仅检查 key 存在，旧请求会误写新请求的占位或结果。
- 比较预期记录内容再更新，确保仍属于当前逻辑所有者。

出处：[Redis 原子更新](../internal/shop/order/cache.go)、[阶段 2 汇报](reports/phase-02-order.md)。

### 2.补3 为什么报告 dropped，为什么不能平均窗口 P99？

- 目标流量发不出去时，完成请求延迟可能很好看，dropped 揭示生成器未达到目标。
- P99 是分布位置，窗口百分位均值不等于整体百分位。
- 总体应合并实际观察的分布计数后计算，真实未达标轮与最后基准一并保留在台账 P2-1。

出处：[压测统计](../internal/loadgen/doc.go)、[阶段 2 汇报](reports/phase-02-order.md)。

### 2.补4 worker 为什么停机后还处理手上消息，最多等多久？

- 停止拉取新消息后，当前消息还要完成业务事务与位点提交。
- 它使用独立但有界的消息处理预算，具体值由 `WORKER_MESSAGE_TIMEOUT` 配置。
- 强制退出仍可能重投，数据库幂等是恢复正确性的保障，不能只依赖优雅停机。

出处：[worker 停机流程](../internal/shop/app/orderworker/app.go)、[阶段 2 汇报](reports/phase-02-order.md)。

### 3.补1 up 与 mysql_up 有什么不同？

- up 表示 Prometheus 能否成功抓取 exporter 的指标端点。
- mysql_up 表示 exporter 是否真正连得上 MySQL。
- 两层都要检查，再用真实查询或业务验证端到端，采集实测见台账 P3-1。

出处：[Prometheus 配置](../deploy/prometheus/prometheus.yml)、[阶段 3 汇报](reports/phase-03-observability.md)。

### 3.补2 同一消息消费两次，销量和指标怎样变化？

- 第一次事务写去重记录并加销量，记录 processed。
- 第二次命中去重，不再次加销量，记录 duplicate，观测仍反映重复确实发生。
- 必须区分业务副作用与处理尝试计数，真实并发重复见台账 P3-10。

出处：[销量服务](../internal/shop/sales/service.go)、[阶段 3 汇报](reports/phase-03-observability.md)。

### 3.补3 trace ID 查不到有哪些原因？

- 可能未采样、还未导出、导出超时或后端重启丢失内存数据。
- 也可能查询时间范围、服务过滤或传播配置错误。
- 对照日志与 SDK 错误，不能据“无 trace”断定请求没发生；旧丢失案例见台账 P3-14。

出处：[追踪初始化](../internal/platform/telemetry/tracing.go)、[阶段 3 汇报](reports/phase-03-observability.md)。

### 3.补4 DNS TXT 那次排障怎样讲？

- 现象是首次或闲置后调用卡住，A 记录可解析，却缺库存端 span。
- 对比 A/TXT 请求发现服务配置 TXT 查询等待，库存客户端与 OTLP 的 gRPC 都受到影响。
- 关闭本项目不使用的服务配置读取，保留主机名解析与原预算，用相同条件复测；全部数字见台账 P3-18 至 P3-23。

出处：[库存客户端](../internal/shop/inventoryclient/inventory_client.go)、[追踪初始化](../internal/platform/telemetry/tracing.go)、[阶段 3 汇报](reports/phase-03-observability.md)。

### 3.补5 为什么重启恢复时间修复前后没明显变化？

- 修复针对额外解析等待，不能消除服务真实停止和启动窗口。
- 同条件单轮实验里，重启恢复仍受到停机影响，不能把微小差别叫优化。
- 闲置重建的对照才体现此次修复范围，应分条件引用台账 P3-20 至 P3-23。

出处：[停机与重连实现](../internal/shop/inventoryclient/inventory_client.go)、[阶段 3 汇报](reports/phase-03-observability.md)。

### 4.补1 配了最大重试次数，是否必然发满？

- 次数只是上限，单次执行耗时、退避等待都会消耗总预算。
- 上游取消、熔断或剩余预算不足时会提前退出。
- 观察重试计数和服务端实际请求，而不是按配置推算实际流量。

出处：[重试器](../internal/platform/resilience/retry.go)、[单次预算测试](../internal/shop/inventoryclient/attempt_test.go)、[阶段 4 汇报](reports/phase-04-chaos.md)。

### 4.补2 消息发送等待超时，为什么不能说没有发送？

- 调用方结束等待与后台批次真正完成不是同一时刻。
- 超时后 Kafka 可能已经接收或随后成功，需要根据确认与消费记录核对。
- 保留不确定结果，不能换订单重发掩盖风险，真实对照见台账 P3-25、P4-2 至 P4-4。

出处：[订单发布](../internal/shop/events/publisher.go)、[阶段 4 汇报](reports/phase-04-chaos.md)。

### 4.补3 回源改成比例，为什么仍分不清冷缓存与雪崩？

- 冷启动和集中到期都可能让大量商品查库，比例本身没有历史背景。
- 告警再检查此前是否已有持续流量，并要求异常持续，降低健康冷启动误报。
- 代价是初始低流量或不同工作集可能不满足前提，不能宣称任意负载都覆盖。

出处：[缓存告警测试](../deploy/prometheus/rules/cache_alert.test.yaml)、[阶段 4 汇报](reports/phase-04-chaos.md)。

### 8.补1 同故障配对为什么还记录随机顺序？

- 同一次故障也随时间变化，前后组的缓存、积压和窗口不完全相同。
- 随机顺序减少固定偏向某一配置，不能消除所有环境差异。
- 事先保存顺序，续跑沿用原记录，避免看到结果后调整顺序或挑选时段。

出处：[配对冻结](../internal/eval/runner/paired.go)、[正式清单](reports/phase-08-formal-manifest.json)、[阶段 8 汇报](reports/phase-08-eval.md)。

### 8.补2 可靠性失败移出质量分母，还应保留什么？

- 原失败记录、尝试费用、未知 token 与金额预留仍保留。
- 补跑使用独立槽位并关联原失败，不能覆盖原数据。
- 格式、推理和预算失败不借此机会重跑挑结果，指定校时例外单独审计。

出处：[可靠性分类](../internal/doctor/store/eval_reliability.go)、[补跑审计](../internal/eval/runner/amendment.go)、[阶段 8 汇报](reports/phase-08-eval.md)。

## 项目层面的问题

### 项目1 最难的地方是什么？

- 可选真实经历之一：服务能运行却缺少观测，沿 span 缺口和 A/TXT 对照定位启动解析等待。
- 另一个是长跑实验的可信度：同故障配对、冻结、不确定费用、时钟中断与恢复都要留审计。
- 讲自己理解最深的一条实际证据链，并承认尚未解决的限制，不背没有发生过的“优化故事”。

出处：[DNS 过程](learn/phase-03-observability.md)、[评测开发回放](learn/phase-08-eval.md)、[阶段 8 汇报](reports/phase-08-eval.md)，数字分别查台账 P3-18 至 P3-23、P8-191。

### 项目2 如果重做，会先改什么？

- 可提出尚未实施的方向：更早建立干净环境验收与完整失败日志，降低旧数据卷掩盖问题的风险。
- 把外部靶场与第二日志后端作为独立验证目标，避免接口设计和泛化效果混为一谈。
- 先补人工评分与真实暴露安全实验，再扩大效果结论；明确这些是下一步建议，不是本阶段成果。

出处：[CI](../.github/workflows/ci.yml)、[阶段 8 汇报](reports/phase-08-eval.md)、历史失败台账 P8-207、S12 台账 P8-204。

### 项目3 为什么选自建靶场，为什么阶段 9 推迟？

- 自建系统能控制原因、恢复动作与标准答案，方便学习和建立可复现实验。
- 代价是题目与作者实现相关，不能据此证明陌生生产系统准确率。
- 用户为尽早投递在 2026-10-30 决定先阶段 10，随后于 2026-11-02 授权恢复阶段 9。OpenSearch 已读取官方 Demo 真实日志，诊断结果已独立成表（P9-77），不能倒写阶段8成绩。

出处：[计划书](项目计划书.md)、[决策记录](decisions.md)、[阶段 4 汇报](reports/phase-04-chaos.md)。

### 项目4 能不能说防提示注入已经验证？

- 不能。S12 的攻击文字没有被医生实际读到，台账 P8-204。
- 已经有工具限制、审批、账号权限和系统隔离测试，证明这些代码边界按测试条件工作。
- 要验证暴露后行为，还需确认攻击材料确实进入工具结果，并检查模型申请与执行；本阶段未补做。

出处：[S12 暴露核对](../internal/eval/runner/injection.go)、[阶段 8 汇报](reports/phase-08-eval.md)、[实际原结果](reports/phase-08-formal-results.md)。

### 项目5 评分失败和评委意见不同，怎样诚实回答？

- 正式 195 份报告中有 14 份评分失败，不能把失败评分删除或当成已知零分，台账 P8-191。
- DeepSeek 评委与 Claude 交叉评分的一致率全量 17/20（85%），去掉 E0 为 12/15（80%），台账 P8-199、P8-201。
- 证据评委项 DeepSeek 多给 11、少给 4，说明尺度有分歧，台账 P8-202；未做人工校准，两个模型可能有共同偏差。
- 原分、原响应和分歧理由都留档，正式冻结后没有临时改标准来取得更好分数。

出处：[评分解析](../internal/eval/judge/grade.go)、[交叉评分](reports/phase-08-cross-scoring.md)、[阶段 8 汇报](reports/phase-08-eval.md)。

### 项目6 费用到底花了多少，怎样核对？

- 正式矩阵分别记 USD $3.221903、CNY ¥6.230244，台账 P8-190；两个币种不相加。
- 美元按项目用量账本计：每次请求实际 token 用量乘以 2026-10-29 核对过的官方单价；平台账单因 Key 与其他用途共用，无法单独核对。
- 未知响应的预留与失败费用仍保留，DeepSeek 余额差 ¥0.769756 尚未归因，见台账 P8-197。
- 阶段 10 演示与检查全用 Mock，真实效果只回看冻结报告，不为面试演示触发付费重跑。

出处：[费用审计](../internal/doctor/store/budget.go)、[阶段 8 汇报](reports/phase-08-eval.md)、[数据台账费用口径](数据台账.md)。

### 项目7 S10 为什么是有价值的失败？

- 带工具的 12 份全部把连接池当根因，台账 P8-149、P8-153、P8-157，说明拿到观测并不自动得到正确因果。
- 网络变慢拉长连接占用，再造成池等待，若只观察池满，就把下游现象当成起因。
- 改进应围绕通用因果对照与缺失证据设计，并全量重跑；不能为 S10 写专门答案后仍引用旧冻结成绩。

出处：[S10 分析](reports/phase-08-failure-analysis.md)、[阶段 8 汇报](reports/phase-08-eval.md)、[查询工具](../internal/doctor/tools/catalog_mysql.go)。

### 项目8 公开仓库怎样保护凭据与原始实验？

- 真实凭据只在环境变量，示例给假值；日志、报错、工具输出和提交前扫描都要检查泄露。
- 结果归档保留真实失败和版本，阶段 8 原始数据库卷不能为了冷启动测试删除。
- 新用户验证使用显式独立 Compose 项目与新卷，测试前后比对原容器和卷；还要分别检查日志采集过滤与共用镜像标签，不能只看项目名。

出处：[凭据读取](../internal/platform/config/config.go)、[Compose](../deploy/docker-compose.yml)、[阶段 8 汇报](reports/phase-08-eval.md)、[项目自身排障](troubleshooting.md)。实际前后对照见台账 P10-8：原 19 个 OpsDoctor 与 2 个 QQ 容器 ID、挂载和状态不变，原卷删除 0；全局 Go 镜像标签更新，旧容器未重建，不声称原数据逐字节未变。

### 项目9 worker 健康探针只 Ping 数据库，为什么还要真实消息验收？

- Ping 证明数据库可达，不能证明 Kafka 就绪、主题可消费或消费者循环正常。
- 受控原版实验中 Compose up 返回 0，随后观察发现 worker 因 Kafka 16 退出，台账 P10-10，说明一次健康快照会漏掉启动后的失败。
- CI 在 healthy 后继续观察、创建真实订单并等待销量追平；修复 `364a889` 默认配置 5/5 新卷真实消费通过，台账 P10-13。
- 数据库总量核对仍有局部错误互相抵消的局限，不能称为任意生产故障下的消息可靠性证明。

出处：[当前 worker 探针](../internal/shop/app/orderworker/app.go)、[真实消费检查](../.github/workflows/ci.yml)、[阶段 10 汇报](reports/phase-10-showcase.md)、[复现日志](reports/phase-10-startup-root-cause.json)、[修复验收](reports/phase-10-startup-fix-validation.json)。

### 项目10 怎样区分早期未复现、已证缺陷和历史未知？

- 原 `f851460` 的前 10 轮未复现，台账 P10-6；停机遥测超时另记，不能拿早期成功或另一个错误编造根因。
- 后来放大内部主题分区并限 CPU，实测协调器尚未加载时 Fetch 返回 Kafka 16，上层直接退出，台账 P10-10。
- `364a889` 精确重试 Fetch 的 14、15、16，最多三次、可取消且复用 Reader；相同条件出现错误后继续消费，默认 5/5 新卷也通过，台账 P10-11、P10-13。
- 这些证据证明已复现缺陷被修复；原 CI 缺 worker/Kafka 日志，不能证明当时与受控实验完全同根。保留证据边界比把未知补写成确定更可靠。

出处：[原失败 CI](https://github.com/Li-Nepenthe/opsdoctor/actions/runs/41447608555)、[Fetch 恢复](../internal/shop/events/consumer.go)、[根因记录](reports/phase-10-startup-root-cause.json)、[修复对照](reports/phase-10-startup-fix-validation.json)、[阶段 10 汇报](reports/phase-10-showcase.md)。

### 项目11 Compose 项目名不同，就代表完全隔离了吗？

- 独立项目名使容器、网络和命名卷按项目区分，但应用里的硬编码过滤不会自动改变。
- Alloy 原来写死 `opsdoctor`，独立项目会漏采自己的日志；`c246734` 改为按最终 `COMPOSE_PROJECT_NAME` 过滤。配置与最终新克隆三服务Loki验证均通过，见台账P10-17及[证据](reports/phase-10-log-isolation.json)。
- 镜像标签在 Docker 中全局共用，构建更新标签不会自动重建原容器，但仍需如实记录这项变化，实测见台账 P10-8。
- 因此验收要同时核对旧容器与卷、新项目真实消费及观测数据，不能把单一命名约定当作全部隔离证据。

出处：[Compose 环境传入](../deploy/docker-compose.yml)、[Alloy 项目过滤](../deploy/alloy/config.alloy)、[阶段 10 汇报](reports/phase-10-showcase.md)、[原环境对照](reports/phase-10-docker-preservation.json)。
