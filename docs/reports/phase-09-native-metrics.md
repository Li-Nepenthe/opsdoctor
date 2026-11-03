# 阶段 9：本轮原生指标间隔与 OT04 等待窗口

本轮实测支持 spanmetrics 约一分钟刷新。OT04 恢复后的基线等待期间，payment 和 checkout 的业务 SERVER 成功计数先出现平台，下一批原生样本增加后，基线通过；错误计数在这些边界间不增加。这与等待下一批已导出业务增量的解释一致，但不能单凭这些指标计算真实下单间隔、去重订单数或证明没有采集延迟。

## 一次快照与原件

本轮 run 为 `phase09-dualmock-20261103T171411Z`。根代理确认 Mock 已结束后，才执行一次只读快照：观察下界 UTC `2026-11-03T17:13:01.793051Z`，统一查询评估时间 `17:56:57.462078Z`，收集完成 `17:56:57.556488Z`。共五个不同的 GET，全部 HTTP 200，重试、失败、warnings、infos、冲突样本均为零。没有 Docker 操作、故障修改或模型调用。

[完整证据 JSON](phase-09-native-metrics.json)内嵌五份原响应文本、各自 SHA256、查询参数、原生样本摘录、恢复及基线原件、分段边界和复现脚本。原响应重新编码为 UTF-8 后，五份 hash 均匹配。完整 summary、分段逐序列分析和脚本原件留在本机 `TEMP/opsdoctor-phase09/native-metrics-research/live-20261103T175657Z-64b5367c`。

原生点使用 `/api/v1/query` 的 `selector[时间范围]` 返回的 matrix；`query_range` 的 step 和离线网格都没有被当成原生间隔。五次查询使用同一评估时间，仍不宣称它们是后端事务式原子快照。

## 实际间隔与固定配置分开核对

| 原生指标 | 序列数 | 样本数 | 相邻间隔最小 / 中位 / 最大（秒） |
|---|---:|---:|---|
| spanmetrics calls | 103 | 3989 | 59.995 / 59.997 / 59.999 |
| 八个容器的 uptime | 8 | 2091 | 9.997 / 10.000 / 10.001 |
| payment 成功路径 SDK counter，按币种分列 | 2 | 74 | 59.995 / 59.997 / 60.000 |

spanmetrics 的相邻间隔共 3886 个，均先在同一完整标签序列内计算；这些序列共享刷新时刻，不能把间隔总数当成独立实验次数。表内三类序列没有观察到负差值。所有 spanmetrics 序列的 `collector_instance_id` 均为 `81fbd4e0-9203-4e1d-ad82-b3fe126497a1`；原生标签原样保留。checkout 未提供 `service_instance_id`，没有替它填造实例号。

固定 Demo commit `dedc0178918e260823323b8d95005a8cb924b007` 的 [Collector 配置](https://github.com/open-telemetry/opentelemetry-demo/blob/dedc0178918e260823323b8d95005a8cb924b007/src/otel-collector/otelcol-config.yml)没有为 `span_metrics` 显式设置间隔；[Collector contrib v0.159.0 默认配置](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.159.0/connector/spanmetricsconnector/factory.go#L34-L38)设置 cumulative 和 60 秒 flush。这是配置预期，表中的间隔则来自本轮原生时间戳，两者没有互相替代。

[观测层配置](https://github.com/open-telemetry/opentelemetry-demo/blob/dedc0178918e260823323b8d95005a8cb924b007/src/otel-collector/otelcol-config-observability.yml)通过 OTLP 推送指标给 Prometheus；[Prometheus 配置](https://github.com/open-telemetry/opentelemetry-demo/blob/dedc0178918e260823323b8d95005a8cb924b007/src/prometheus/prometheus-config.yaml)中的 scrape interval 不能拿来证明这些 OTLP 指标的原生周期。uptime 的十秒序列也没有混入 spanmetrics 的一分钟统计。

## OT04 等待的原始数值

OT04 post-cool baseline 的整体起止为 `17:47:44.172507–17:48:21.465288Z`，整体耗时 37.2926579 秒；其中健康轮询单独为 `17:47:46.4116338–17:48:21.4620963Z`，耗时 35.0503469 秒，共八轮、四十次查询。整体检查还包括资源、开关与开放期，不能把两个耗时混为一个。基线原件已内嵌在证据 JSON。

下面是同一 Collector 的原生累计值。SERVER 成功行实际状态为 `STATUS_CODE_UNSET`；ERROR 列也是累计值，不是本次新增错误数。

| 原生时间（UTC） | payment Charge 成功 | checkout PlaceOrder 成功 | payment Charge ERROR | checkout PlaceOrder ERROR |
|---|---:|---:|---:|---:|
| 17:46:20.975 | 76 | 76 | 2 | 5 |
| 17:47:20.973 | 76 | 76 | 2 | 5 |
| 17:48:20.970 | 78 | 78 | 2 | 5 |

前两个边界之间成功计数不变，下一边界两条成功序列各增加 2；ERROR 计数都不增加。原生下一批样本时间比实际健康轮询结束早约 0.492 秒。checkout 的 `EmptyCart`、支付 CLIENT 和 PlaceOrder SERVER 三条 ERROR 序列在这一焦点段也都保持原值。

按已记录的健康轮询起止 UTC，离线查看各自 `(t-120,t]` 中的真实原生点：payment 业务成功、业务 ERROR 以及 checkout 业务成功、业务 ERROR 在两端都各有两个点。因此这次焦点里的平台不能解释为“根本没有足够原生点”。OT03 的未外推 calls 差值从 0 变为 2，errors 差值保持 0；OT04 冻结查询覆盖 checkout 全服务，calls 差值从 0 变为 26，errors 差值也保持 0。26 包含多层调用 span，不能叫 26 个订单。起止 UTC 是观察记录时刻，并非逐条请求的 Prometheus 服务端评估时间；这里没有伪造八轮的精确查询时点。

独立 SDK 成功路径 counter 也出现平台，但导出时刻与 spanmetrics 不同：

| SDK 原生时间（UTC） | USD | CAD |
|---|---:|---:|
| 17:46:22.143 | 70 | 6 |
| 17:47:22.142 | 70 | 6 |
| 17:48:22.139 | 73 | 6 |

[固定 payment 源码](https://github.com/open-telemetry/opentelemetry-demo/blob/dedc0178918e260823323b8d95005a8cb924b007/src/payment/charge.js)在成功处理路径增加该 counter；合成调用也可能计入。SDK 的增量 3 与业务 span 的增量 2 来自不同采样边界，不能硬凑相等，更不能充当最终订单总数。[固定负载脚本](https://github.com/open-telemetry/opentelemetry-demo/blob/dedc0178918e260823323b8d95005a8cb924b007/src/load-generator/locustfile.py)采用带权随机任务与等待；本轮原生平台支持已观测完成计数较稀疏的解释，不能据配置或平台反推每笔实际请求的时间。

## 分段与结论边界

五份本 run 恢复证明的 original 原始 JSON 字节 SHA 全部匹配。离线分析将注入意图至恢复确认、恢复后两分钟、恢复 original 后再等待两分钟的区间分开；恢复文件的 run、场景、轮次也与路径核对。恢复 original 本身不证明所有开关 off，排除回看污染也不等于整段连续健康。

离线五秒网格只统计真实原生点数，不替代 runner 的真实轮询或重新评分。Prometheus 官方说明 [range selector 左开右闭](https://prometheus.io/docs/prometheus/latest/querying/basics/#range-vector-selectors)，[increase 会处理重置并外推](https://prometheus.io/docs/prometheus/latest/querying/functions/#increase)。本报告的相邻 counter 差没有做外推或重置补偿，不使用 `increase()` 小数冒充原始交易数。

这是有限窗口内的指标证据，没有逐订单标识，也不证明端到端无丢失。旧历史的 OT04 对应开关 off 口径仍包含两个 checkout CLIENT POST 错误点，不能因为本轮焦点是零增量，就覆盖[原历史失败](phase-09-historical-baseline-replay.md)或把它们改写成全部 0/0。本报告不计算诊断准确率，不代替根代理的 Mock 完整性或付费放行检查。
