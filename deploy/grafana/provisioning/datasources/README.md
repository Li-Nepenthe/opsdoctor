# deploy/grafana/provisioning/datasources：数据源

[sources.yml](sources.yml) 配置了三个数据源，地址都是容器之间互相访问用的内部地址：

| 数据源 | 地址 | 用来查什么 |
|---|---|---|
| Prometheus | `http://prometheus:9090` | 指标（默认数据源） |
| Jaeger | `http://jaeger:16686` | 调用链 |
| Loki | `http://loki:3100` | 日志 |

## 调用链和日志之间的跳转

这里还配置了两个方向的跳转，排查问题时很有用：

- **从调用链跳到日志**：在 Jaeger 数据源里看一条 trace 时，可以直接查看这条 trace 对应的日志。
- **从日志跳到调用链**：Loki 会从每条日志的 JSON 里识别出 `trace_id`，旁边显示一个链接，点一下就能打开这条调用链。

这里不保存任何密码或令牌。
