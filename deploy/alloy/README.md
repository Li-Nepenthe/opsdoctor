# deploy/alloy：日志收集

各服务只把日志打印到标准输出，自己不负责把日志送到哪里。Alloy 负责收集：它通过 Docker 读取容器的输出，送到 Loki。

[config.alloy](config.alloy) 做了三件事：

1. **只找本项目的容器**：只看带有 Compose 项目标签 `opsdoctor` 的容器，不会收集你电脑上其他项目的日志。
2. **只保留 5 个 Go 程序的日志**：订单服务、库存服务、order-worker、压测工具、医生。Prometheus 这类监控组件自己的日志不收集，需要时用 `docker compose logs` 查看。
3. **加上标签**：给每条日志加上 `service`（哪个服务）和 `level`（日志级别）两个标签，方便查询。trace_id 不做成标签，原因见 [loki/README.md](../loki/README.md)。
