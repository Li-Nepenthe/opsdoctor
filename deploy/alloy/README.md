# deploy/alloy：日志收集

各服务只把日志打印到标准输出，自己不负责把日志送到哪里。Alloy 负责收集：它通过 Docker 读取容器的输出，送到 Loki。

[config.alloy](config.alloy) 做了三件事：

1. **只找本项目的容器**：Compose 将最终项目名通过 `COMPOSE_PROJECT_NAME` 传给 Alloy，配置用这个值匹配容器的项目标签。默认名是 `opsdoctor`；使用 `-p opsdoctor-fresh` 时，就只收集 `opsdoctor-fresh` 的日志，不会转而读取其他项目。
2. **只保留 5 个 Go 程序的日志**：订单服务、库存服务、order-worker、压测工具、医生。Prometheus 这类监控组件自己的日志不收集，需要时用 `docker compose logs` 查看。
3. **加上标签**：给每条日志加上 `service`（哪个服务）和 `level`（日志级别）两个标签，方便查询。trace_id 不做成标签，原因见 [loki/README.md](../loki/README.md)。

项目名由 Compose 的内置插值提供，见 [Compose 项目名说明](https://docs.docker.com/reference/compose-file/version-and-name/)；Alloy 通过 [`sys.env`](https://grafana.com/docs/alloy/latest/reference/stdlib/sys/) 读取它。直接运行 Alloy 时也要显式提供这个变量。

CI 的独立冷启动验收会给真实订单附加唯一、强制采样的 `traceparent`，等消费追平后，再有限等待 Loki 中出现同一 trace 的订单、库存、消费者日志。这样可以发现“容器都启动了，但日志仍按旧项目名过滤”的接线错误。
