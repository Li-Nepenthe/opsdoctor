# 调用链后端

收 OTLP、保存有限数量的内存 trace、提供 UI；不永久保存业务数据。

- [jaeger.yml](jaeger.yml)：接收端点、内存限额和查询服务。

当前固定 `2.20.0`，这是用户批准的单项版本例外。Grafana 仍使用的 `/api/services` 在 `2.22.0` 返回 404。对照证据、上游问题和解除条件见 [decisions](../../docs/decisions.md#jaeger-compatibility)。以后升级时同时检查 Grafana，以及阶段 5 的 Jaeger 查询适配器。

建议先读配置旁的中文注释，再对照阶段 3 学习材料运行查询。
