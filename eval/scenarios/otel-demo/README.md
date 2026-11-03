# OpenTelemetry Demo 的阶段 9 场景

本目录只给宿主机评测器读取，不能挂载到医生容器，也不能放进系统说明书或通用手册。每个 YAML 中的 `symptoms` 是给医生的现象描述；开关名、标准答案和证据里程碑只给注入器与评委。

场景来自 OpenTelemetry Demo **3.1.0**，固定源码提交为 `dedc0178918e260823323b8d95005a8cb924b007`。使用上游自带功能制造现象，不修改医生的提示 v8.0、排障手册 v1.1 或 rubric-v1.1。

## 文件与标准答案来源

全部开关的公开解释见[官方 Feature Flags 文档](https://opentelemetry.io/docs/demo/feature-flags/)，版本内的名称、变体和匹配条件见[3.1.0 开关配置](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/src/flagd/demo.flagd.json)。官网随上游更新，因此下面同时给出固定版本实现，防止把后来增加的能力算入本次评测。

| 文件 | 上游开关和变体 | 标准答案与固定源码 |
|---|---|---|
| [OT01.yaml](OT01.yaml) | `adFailure=on` | ad 的 GetAds 约十分之一请求返回 UNAVAILABLE；[AdService.java](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/src/ad/src/main/java/oteldemo/AdService.java) |
| [OT02.yaml](OT02.yaml) | `cartFailure=100%` | EmptyCart 选择不可达的存储，其他购物车操作仍走正常存储；[CartService.cs](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/src/cart/src/services/CartService.cs)、[存储装配](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/src/cart/src/Program.cs) |
| [OT03.yaml](OT03.yaml) | `paymentFailure=100%` | 已到达 payment 的支付处理返回 Invalid token 错误；[charge.js](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/src/payment/charge.js)、[index.js](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/src/payment/index.js) |
| [OT04.yaml](OT04.yaml) | `paymentUnreachable=on` | checkout 的支付客户端使用错误地址，根因组件标为 checkout->payment；[chargeCard](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/src/checkout/main.go) |
| [OT05.yaml](OT05.yaml) | `productCatalogFailure=on`，保留 targeting | 指定商品的 GetProduct 在查库之前返回 Internal；[商品处理](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/src/product-catalog/main.go)、[指定商品条件](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/src/flagd/demo.flagd.json) |

OT02 的 checkout 实现会忽略清空购物车失败，因此本场验证 cart 错误 span，不把“结账必须返回 HTTP 500”作为通过条件。OT04 与 OT03 用来区分“目标服务处理后报错”和“调用方用了错误地址”。OT05 保留公开配置中的商品匹配条件，只修改 `targeting.if` 的命中分支；删除整个 targeting 会扩大故障范围，不属于本场景。

## 注入、现象确认与恢复

注入器先读回并保存完整上游配置，然后只改变选定开关；恢复时写回保存的完整配置。上游接口见[路由](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/src/flagd-ui/lib/flagd_ui_web/router.ex)和[控制器](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/src/flagd-ui/lib/flagd_ui_web/controllers/feature_controller.ex)：GET `/api/read`，POST `/api/write`，写入正文放在 `data` 字段。写入是异步的，收到 HTTP 成功后还要读回核对。

`symptom.promql` 只查服务的错误调用计数，不读取开关、评测器记录或答案字段。指标名称来自上游[默认看板](https://github.com/open-telemetry/opentelemetry-demo/blob/3.1.0/src/grafana/provisioning/dashboards/demo/demo-dashboard.json)中的 `traces_span_metrics_calls_total`。现象必须在 `within` 指定窗口内真实出现，才能创建诊断；未出现时记录失败并停止，不能把开关写入成功算作故障成功。

`healthy_promql` 同时要求最近窗口存在实际调用、错误调用增量为零。没有流量或没有指标时不能当作恢复健康。恢复后仍应遵守评测器的冷却与时钟检查，不能让上一场的错误混入下一场。

上述 PromQL 是按固定版本公开观测配置编写的条件；部署验收还要核对真实返回的指标、服务标签和日志字段。本文件不是已经完成故障注入或评测的证明，实际结果以阶段 9 运行记录和数据台账为准。

## 结果与数据隔离

本目录使用独立 `OT` 场景编号和阶段 9 新批次。E1、E2 的结果按医生实际模型分开列出，与阶段 8 的 shop 数据分表。单元测试和 CI 只使用 Mock；实际评测遵循用户授权的美元与人民币账户上限，评委继续使用 DeepSeek 和 rubric-v1.1。
