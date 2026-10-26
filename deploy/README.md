# deploy：本机部署

这里放在本机用 Docker 运行整个项目所需的全部配置。核心是 [docker-compose.yml](docker-compose.yml)：一条命令就能启动数据库、消息队列、各个服务和整套监控组件。其余子目录放各组件自己的配置文件。

这里不放 Go 代码，也不放真实密码。密码写在仓库根目录的 `.env` 里（Git 会忽略这个文件），启动时由 Compose 读进来。

## 里面有什么

| 目录 | 作用 | 出现于 |
|---|---|---|
| [docker](docker/README.md) | 所有 Go 服务共用的镜像构建文件 | 阶段 1 |
| [redis](redis/README.md) | 从环境变量生成业务与医生的账号权限文件 | 阶段 5 |
| [mysql](mysql/README.md) | MySQL 第一次启动时执行的建库、建账号脚本 | 阶段 0 |
| [prometheus](prometheus/README.md) | 指标采集，以及告警规则 | 阶段 3 |
| [alertmanager](alertmanager/README.md) | 把告警分组，推送给告警接收器 | 阶段 3 |
| [jaeger](jaeger/README.md) | 接收和查询调用链 | 阶段 3 |
| [loki](loki/README.md) | 存储日志 | 阶段 3 |
| [alloy](alloy/README.md) | 从各容器收集日志，送到 Loki | 阶段 3 |
| [grafana](grafana/README.md) | 看板：把指标、调用链、日志放在一个页面里看 | 阶段 3 |
| [toxiproxy](toxiproxy/README.md) | 转发三条真实链路，供宿主机实验工具调整延迟 | 阶段 4 |

## 从哪里开始读

先读 `docker-compose.yml`。每个服务重点看四样东西：用哪个镜像、开放哪些端口、怎么判断已经就绪（`healthcheck`）、依赖哪些服务（`depends_on`）。启动命令见根目录 [README](../README.md)。
