# deploy/grafana：看板

Grafana 是看数据的界面：它本身不存数据，而是去 Prometheus 查指标、去 Jaeger 查调用链、去 Loki 查日志，把它们显示在同一个页面里。

浏览器打开 http://127.0.0.1:3000，用户名 `admin`，密码是你本机 `.env` 里的 `GRAFANA_ADMIN_PASSWORD`。

这里的配置让 Grafana 启动时自动准备好一切，不需要在页面上手动添加：

| 目录 | 内容 |
|---|---|
| [provisioning](provisioning/README.md) | 启动时自动加载的配置：三个数据源，以及看板从哪个目录加载 |
| [dashboards](dashboards/README.md) | “shop 总览”看板 |

管理员密码不保存在仓库里，启动时从环境变量读取。
