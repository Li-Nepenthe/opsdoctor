# deploy/prometheus：指标采集和告警规则

Prometheus 负责收集指标。它每 10 秒主动访问一次各服务的 `/metrics` 地址，把取回的数字存起来（这个动作叫“采集”，英文 scrape），再每 10 秒按告警规则检查一次。

| 文件 | 内容 |
|---|---|
| [prometheus.yml](prometheus.yml) | 采集哪些地址、多久采集一次、告警发给谁（Alertmanager） |
| [rules/shop.yml](rules/shop.yml) | 告警规则，见 [rules/README.md](rules/README.md) |
| [rules/cache_alert.test.yaml](rules/cache_alert.test.yaml) | 用合成时间线检查缓存告警的冷启动与持续异常边界 |
| [rules/goroutine_alert.test.yaml](rules/goroutine_alert.test.yaml) | 区分启动建立后台任务与持续增长，检查实例匹配和重启边界 |

## 采集了哪些目标

一共 8 个，分三组：

- **shop**：靶场的 4 个程序，订单服务、库存服务、order-worker、压测工具。它们的管理端口直接提供 `/metrics`。
- **doctor**：医生自己的进程指标，和靶场任务分开。
- **middleware**：MySQL、Redis、Kafka 本身不提供 Prometheus 格式的指标，所以各配了一个 exporter（采集器）。exporter 连上对应的组件，读取它的状态，再翻译成 Prometheus 能读的格式。

打开 http://127.0.0.1:9090/targets 可以看到这 8 个目标是否都是 UP。注意：UP 只说明 Prometheus 能访问到 exporter，不说明 exporter 能连上 MySQL。后者要看 `mysql_up` 这类指标是不是 1。
