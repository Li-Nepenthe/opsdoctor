# deploy/loki：日志存储

Loki 负责保存和查询日志。各服务的日志由 Alloy 收集后送到这里，你可以在 Grafana 里用 LogQL 查询，比如按 trace ID 找出一笔请求在三个服务里的全部日志。

[loki.yml](loki.yml) 的设置：

- 单副本，存在本地文件里，只保留最近 48 小时的日志，防止长时间运行把磁盘占满。

## 为什么 trace_id 不做成标签

Loki 用“标签”给日志建索引，比如 `service="order-service"`。每一种不同的标签组合，都会单独存成一组数据。

服务只有几个，用作标签没问题；但 trace ID 每个请求都不一样，做成标签的话，标签组合会无限增多，存储和查询都会越来越慢。所以 trace_id 只放在日志的 JSON 正文里，查询时再解析：

```logql
{service="order-service"} | json | trace_id="填入 trace ID"
```

指标的标签也是同样的道理，阶段 3 学习材料里讲“标签基数”的那一节有详细解释。
