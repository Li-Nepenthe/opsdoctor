# deploy/jaeger：调用链

Jaeger 负责接收、保存和查询调用链。各服务把 span 通过 OTLP 协议发到它的 4317 端口；你在浏览器打开 http://127.0.0.1:16686 就能按服务、接口或 trace ID 查找调用链，看到一次请求每一步花了多久。

[jaeger.yml](jaeger.yml) 的设置：

- **只存在内存里，最多 5000 条 trace。** 本机学习够用，不占磁盘。代价是旧的 trace 会被新的挤掉，Jaeger 重启后全部清空。
- **默认只记录十分之一的请求**（采样）。所以拿着一个请求的 trace ID，不一定能在 Jaeger 里查到它。想确保某个请求被记录，可以在请求头里带上 `traceparent` 并标记为需要采样，阶段 3 学习材料里有例子。

## 为什么固定在 2.20.0

项目原则上使用各组件的最新稳定版，Jaeger 是唯一的例外。实测 Jaeger 2.22.0 去掉了 Grafana 还在使用的 `/api/services` 接口（返回 404），Grafana 就查不了调用链了；同样的配置连 2.20.0 正常。你同意后，固定在 2.20.0。

对照证据和解除条件记录在 [decisions](../../docs/decisions.md#jaeger-compatibility)。以后升级 Jaeger 时，要同时检查 Grafana，以及阶段 5 医生查询 Jaeger 的代码。
