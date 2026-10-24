# deploy/grafana/provisioning：自动加载的配置

Grafana 启动时会读这个目录，自动创建数据源和看板（这个机制叫 provisioning）。这样每次全新启动，看到的都是同样的配置，不用手动去页面上点。

| 目录 | 内容 |
|---|---|
| [datasources](datasources/README.md) | 三个数据源：Prometheus、Jaeger、Loki，以及调用链和日志之间的互相跳转 |
| [dashboards](dashboards/README.md) | 告诉 Grafana 去哪个目录加载看板文件 |
