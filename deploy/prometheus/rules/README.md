# deploy/prometheus/rules：告警规则

[shop.yml](shop.yml) 定义了 12 条告警规则。P99 表示百分之九十九的请求都能在这个时间内完成；它变高，说明较慢的那部分请求正在变慢。

| 告警 | 什么时候触发 |
|---|---|
| OrderLatencyHigh | 下单接口的 P99 耗时超过 1 秒，持续 1 分钟 |
| HTTPErrorRateHigh | 某个服务返回 5xx 错误的比例超过 5%，持续 1 分钟 |
| ConsumerLagHigh | order-worker 积压的消息超过 1000 条，持续 2 分钟 |
| GoroutinesGrowing | 进程已启动超过 90 秒，goroutine 数量仍每秒增加超过 1 个，持续 1 分钟 |
| ServiceUnavailable | 某个服务的管理端口连续 30 秒采集不到 |
| InventoryLatencyHigh | 库存服务处理请求的 P99 超过 200ms，持续 30 秒 |
| OrderListLatencyHigh | 订单列表 P99 超过 200ms，持续 30 秒 |
| DatabasePoolWaiting | 每秒超过一次请求需要等待数据库连接，持续 30 秒 |
| InventoryCallFailures | 库存调用失败比例超过 10%，持续 30 秒 |
| InventoryRequestsAmplified | 每秒重试超过 100 次，且库存收到的请求超过 HTTP 入口的两倍，持续 30 秒 |
| RowLockWaiting | MySQL 每秒累计行锁等待超过 10ms，持续 30 秒 |
| CacheLoadsHigh | 商品回源比例超过 35%，当前和两分钟前都有足够流量，条件持续 30 秒 |

## 只写症状，不写原因

每条规则描述的都是“看得到的症状”：慢了、错误多了、消息积压了。没有一条写的是“数据库缺索引”这种原因。

因为同一个症状可能有很多原因。比如消息积压，可能是 order-worker 停了，也可能是数据库变慢了，还可能是消息格式坏了。告警只负责发现“有问题”，找出原因是医生的工作。如果规则里直接写原因，就等于把一个没有验证过的猜测当成了结论。

## 为什么要“持续一段时间”

每条规则都有一个 `for`，表示条件要持续成立这么久才触发告警。这样能过滤掉偶尔的一次抖动。代价是发现得晚一些：比如 ConsumerLagHigh 至少要积压到 1000 条、再持续 2 分钟才会报。

## 为什么缓存告警还要看之前的流量

回源就是缓存里没有数据，需要去数据库查询。停流超过缓存的保留时间后，恢复访问会自然带来一波回源。原来的规则只要每秒回源超过 50 次就报警，因此把这种正常填充也当成了故障。

现在先算一个比例：最近一分钟的商品回源次数，除以同一分钟的商品 HTTP 请求数。例如 100 次商品请求需要查数据库 40 次，回源比例就是 40%。分母只取 `GET /products/:id`。缓存内部会再次检查同一个键，这些检查不能当作新的用户请求。

比例超过 35% 后，还要检查当前一分钟、以及两分钟前的那一分钟，商品请求速率是否都超过每秒 20 次。`offset 2m` 就是把查询窗口向前移动两分钟。这个前提让停流后的正常填充有时间完成，也避免几个请求就让比例大幅跳动。全部条件持续三十秒，告警才从观察中的 `pending` 变成正式触发的 `firing`。

这条规则关注的是已有流量时回源负担上升。冷启动最初几分钟、低流量和大工作集需要单独评估；请求变慢或失败仍由耗时和错误率规则观察。同键并发回源计数只留给后续诊断，告警不读取它，也不读取 singleflight 配置。

## 为什么 goroutine 告警先等进程启动

goroutine 是 Go 程序里同时做事的小任务。进程启动后，要逐步建立连接、启动后台循环，再接收流量。这时数量上涨是正常现象，不能直接说任务泄漏了。

`process_start_time_seconds` 记录进程启动的时间。用现在的时间减去它，就知道这个进程已经运行了多久。规则只在运行超过 90 秒后，才开始累计那一分钟的持续增长。启动时间和数量必须来自同一个服务实例；别的实例已经运行很久，不能替这个新实例通过判断。

这次本机冷启动中，订单服务增长超过门槛的样本出现在启动约 48–65 秒处，随后恢复稳定。因此先留出 90 秒，再用真实重启和 S09 检验这段等待是否合适。这是本项目在当前机器上的取值，不是所有 Go 服务都通用的常数。保护期内确实可能漏掉早期泄漏，所以还要验证持续泄漏能在三分钟内被发现。

整套环境冷启动时，`OrderLatencyHigh` 仍可能触发。MySQL 的缓冲池是把常用数据页放在内存里的地方；刚重启时，还没有装入足够的数据，请求需要更多地读磁盘，下单可能真的变慢。这条规则反映用户实际等待的时间，保持原来的门槛和持续时间。不要为了让冷启动没有任何告警而把它屏蔽。

## 怎样验证规则

[cache_alert.test.yaml](cache_alert.test.yaml) 用人工构造的计数时间线检查规则边界：健康流量放大、持续异常、停流恢复、少量请求和完全无流量。它只验证判断逻辑，真实数据另记在[数据台账](../../../docs/数据台账.md)。生产配置只加载 `*.yml`，测试文件使用 `.yaml`，不会被误加载。

[goroutine_alert.test.yaml](goroutine_alert.test.yaml) 则检查启动后趋于稳定、持续泄漏、旧进程开始增长、启动时间来自错误实例、进程重启五种情况。

从仓库根目录执行：

```powershell
docker compose --env-file .env -f deploy/docker-compose.yml exec -T prometheus promtool test rules /etc/prometheus/rules/cache_alert.test.yaml
docker compose --env-file .env -f deploy/docker-compose.yml exec -T prometheus promtool test rules /etc/prometheus/rules/goroutine_alert.test.yaml
```

你应该看到 `SUCCESS`。CI 也运行同一条命令。
