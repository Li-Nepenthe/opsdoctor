# Jaeger 契约样例

这两份 JSON 来自本机固定的 Jaeger 2.20.0，采集于阶段 5 的 2026-10-26 UTC。搜索条件是 `service=order-service`、`operation=POST /orders`，从真实搜索结果中选择一条，再按同一个 ID 请求详情。

`search.json` 是搜索外壳加所选的一条 trace；`detail.json` 是详情返回。它包含三个服务、24 个 span。仅把 `db.connection_string` 替换为说明文字，避免把连接细节带进仓库。其余结构保留，包括字符串和整数属性、process 引用和父子关系。

它们用于检查适配器能否读懂接口，不是性能基准。升级 Jaeger 时，需要重新取得同类真实样例并回归搜索与详情，不能只改测试里的版本文字。
