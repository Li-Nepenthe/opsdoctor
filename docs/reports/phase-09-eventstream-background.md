# EventStream 背景周期与一次受控开关观察

本次有限观察支持默认的约 600 秒 EventStream deadline 背景机制。在已捕获的四个当前客户端实例中，没有观察到本次 `adFailure` 开启、恢复动作引起提前结束或额外短流。不能据此证明所有进程、未导出的流或未来切换都没有异常，也不能把它当作医生诊断成绩。

本轮没有调用医生模型或评委。root 只控制上游原生 `adFailure`，其他开关与服务保持原状；观察器只发送 Jaeger GET 请求，不修改开关、配置或容器。完整数字与时序见[公开分析 JSON](phase-09-eventstream-background.json)，动作与恢复校验见[root 控制记录](phase-09-eventstream-controlled-20261103T072744Z.json)及[逐次原始样本](phase-09-eventstream-controlled-20261103T072744Z.jsonl)。root 的开关、GetAds 症状与恢复验证另见[受控验证摘要](phase-09-ot01-controlled-validation.json)，不与这里的 Jaeger 轮次混算。本报告不自行分配数据台账编号。

## 观察方法与时间

下列时间均为 2026-11-03 UTC，动作时间采用 root 记录的 `confirmed_at_utc`：

| 事件 | 时间 |
|---|---|
| 开启 `adFailure`，读回确认 | 07:27:44.659363 |
| Jaeger 定时观察开始 | 07:28:08.862299 |
| 恢复原始完整开关，读回确认 | 07:30:44.674459 |
| 最后一次查询完成 | 07:42:09.025240 |
| 观察进程结束 | 07:42:15.045077 |

观察按单调时钟持续 **846.194369 秒**，共 **29 轮、146 次 GET**；客户端每 30 秒查询，flagd 服务端每 60 秒另查。每次查询回看最近两小时、上限 1000 条 trace；本轮没有查询达到该上限，HTTP/API 失败为 0，146 份原始响应的 SHA-256 复核不匹配为 0。

实际 API 使用 `http://127.0.0.1:19686/jaeger/ui/api/...`。固定 Demo 的[Jaeger 配置](https://github.com/open-telemetry/opentelemetry-demo/blob/dedc0178918e260823323b8d95005a8cb924b007/src/jaeger/config.yml#L36)设有 `/jaeger/ui` 前缀；先前裸 `/api/services` 的 404 与带前缀的 200 均留有原件。本观察使用正确前缀，不代表医生 trace 工具的接入修复已经由本报告复验。

首次定时采样在开启动作之后约 24 秒。查询能取回仍被保留的已结束流，但不能补成完整的动作前实时监测。为抵抗 Jaeger 淘汰，所有响应都独立保存，每个 trace 对象出现新版本也单独保留；最终有 531 个 trace ID、545 个对象版本。流以 **trace ID + span ID** 标识，不能把重复 trace ID 当成同一条流。

## 四个当前客户端实例

每个服务各保存四条完整 client/v2 span，全部为 gRPC code `4`、OTel `ERROR`。它们包含首次采样取回的历史样本，**不是在这十四分钟中新结束了四条**。实例标识、每条起止时间、状态和消息事件在 JSON 中逐项列出，未取得的旧实例历史不作推断。

| 服务 | `service.instance.id` | 完整流时长范围（秒） | 相邻起点间隔范围（秒） | 结束到下一起点范围（秒） |
|---|---|---:|---:|---:|
| ad | `d04ba161-a7f8-45d9-80b8-85aacd6adbc1` | 600.000959–600.001850 | 604.967785–604.974839 | 4.966197–4.973010 |
| fraud-detection | `63ee6d48-2cef-42ea-9eb9-0c64af3f45da` | 600.002273–600.003236 | 604.969370–604.975266 | 4.966134–4.972993 |
| recommendation | `8d88b123-3de6-4978-975c-19645daf4077` | 599.963547–599.973969 | 611.969835–611.974305 | 11.999249–12.000372 |
| payment | `669991a1-78b4-41a5-92a5-c457f490cf72` | 600.000018–600.000980 | 604.969000–604.971000 | 4.968020–4.970596 |

### 为什么 07:30、07:40 的错误不能直接归因于切换

ad 在开启期间结束的流，早于动作数分钟就已经开始；跨恢复动作的下一条流也持续到完整 deadline。以下是关键交叉记录，完整十六条原件见[客户端 span 文件](phase-09-eventstream-client-spans.json)：

| 服务与跨越动作 | span 开始 | span 结束 | 时长（秒） | code | 动作附近的消息事件 |
|---|---|---|---:|---:|---|
| ad：跨开启 | 07:20:07.192146 | 07:30:07.193975 | 600.001829 | 4 | 07:27:45.238553 RECEIVED |
| ad：跨恢复 | 07:30:12.166985 | 07:40:12.167944 | 600.000959 | 4 | 07:30:45.201015 RECEIVED |
| fraud-detection：跨开启 | 07:20:11.234936 | 07:30:11.237209 | 600.002273 | 4 | 07:27:45.239217 RECEIVED |
| fraud-detection：跨恢复 | 07:30:16.210202 | 07:40:16.212706 | 600.002504 | 4 | 07:30:45.201315 RECEIVED |
| recommendation：跨开启 | 07:20:34.046179 | 07:30:34.020148 | 599.973969 | 4 | 没有对应 message 事件，不据此判定未收到配置 |
| payment：同一流跨开启与恢复 | 07:21:25.621000 | 07:31:25.621404 | 600.000404 | 4 | 没有对应 message 事件，不据此判定未收到配置 |

消息事件只保存接收时间和消息编号，没有 payload，因此不能凭时间直接解码成某个 flag 的内容。它们表明这些已捕获流在消息事件之后仍继续存活，未在消息处提前结束。短开启窗口恰好覆盖自然 deadline 时，窗口里的错误完成数可能增加；应检查原始 start、duration 和重连间隔，不能只按错误出现时刻归因。

## 固定上游源码如何解释观察

Demo 固定在 `3.1.0`、提交 `dedc0178918e260823323b8d95005a8cb924b007`。[ad 依赖](https://github.com/open-telemetry/opentelemetry-demo/blob/dedc0178918e260823323b8d95005a8cb924b007/src/ad/build.gradle#L58)与[fraud-detection 依赖](https://github.com/open-telemetry/opentelemetry-demo/blob/dedc0178918e260823323b8d95005a8cb924b007/src/fraud-detection/build.gradle.kts#L52)都固定 Java flagd provider `0.14.1`；[ad 的初始化](https://github.com/open-telemetry/opentelemetry-demo/blob/dedc0178918e260823323b8d95005a8cb924b007/src/ad/src/main/java/oteldemo/AdService.java#L113)只开启全局遥测，没有在该代码中覆盖流时限。

provider `0.14.1` 对应提交 `d69584b44cc98ef9a869a00b0ce2ab63521106e1`。[Config.java](https://github.com/open-feature/java-sdk-contrib/blob/d69584b44cc98ef9a869a00b0ce2ab63521106e1/providers/flagd/src/main/java/dev/openfeature/contrib/providers/flagd/Config.java#L20)默认流 deadline 为 600000 毫秒、最大重试退避为 5000 毫秒；[RpcResolver](https://github.com/open-feature/java-sdk-contrib/blob/d69584b44cc98ef9a869a00b0ce2ab63521106e1/providers/flagd/src/main/java/dev/openfeature/contrib/providers/flagd/resolver/rpc/RpcResolver.java#L334)重新发起流时设置 deadline，错误或完成后安排重连。这与 ad、fraud 的约 600 秒流时长、约 5 秒重连间隔相符。该文件的配置变化处理只清理缓存并发送事件，没有直接重启流；其他语言客户端的间隔按实测保留，不声称它们使用完全相同的 Java 实现。

**不能写成“flagd 服务端默认设置了 600 秒超时”。** 固定 flagd `v0.16.0`、提交 `80b9e9548163c1adbd28d45ca52364956e7fa08f` 的[启动参数](https://github.com/open-feature/flagd/blob/80b9e9548163c1adbd28d45ca52364956e7fa08f/flagd/cmd/start.go#L95)默认 `--stream-deadline=0`，即不另设服务端时限；[Demo 的启动命令](https://github.com/open-telemetry/opentelemetry-demo/blob/dedc0178918e260823323b8d95005a8cb924b007/compose.yaml#L697)也没有提供这个参数。[EventStream 实现](https://github.com/open-feature/flagd/blob/80b9e9548163c1adbd28d45ca52364956e7fa08f/flagd/pkg/service/flag-evaluation/flag_evaluator_v2.go)在 context deadline 超时时返回 code 4，并使用 `server-side timeout` 文案；这个 context 也可承接客户端的 deadline，错误文字本身不能证明是独立服务端定时器。

## 服务端与协议版本分列

| 捕获类别 | span 数 | 可见状态 | 边界 |
|---|---:|---|---|
| 四服务 client / v2 / gRPC | 16 | code 4 共 16，OTel ERROR 共 16 | 按已识别的当前客户端实例筛选 |
| flagd server / v2 / gRPC | 16 | code 4 共 9，code 0 共 7 | 服务端缺 `service.instance.id`，不能伪造旧新实例对照 |
| flagd server / v1 / connect_rpc | 518 | 没有 gRPC code 或 OTel status 标签 | 不把“缺少错误标签”写成 518 条均已证明成功，也不混入 v2 deadline 数 |

客户端 code 4 不要求每个配对 server span 也必须是 code 4；本次真实记录包含不同服务端结果，完整分布原样保留，不强行抹平。

## 原件、指纹与结论边界

- [公开分析 JSON](phase-09-eventstream-background.json)保存每条客户端的归属、起止、duration、status、动作附近消息时间、上游固定源链接及本机归档指纹。
- [公开客户端原件](phase-09-eventstream-client-spans.json)为 259017 字节，SHA-256 为 `5260af19cc7ef20d90e17954c3d2724d6dc4d42daf57c6cdc5d771ab3a878e2c`。十六条 span 和各自 process 字段从原 Jaeger 对象逐字段保留，JSON 重新编码；公开文件指纹与原 HTTP 响应指纹分别记录，不把它们当成同一份字节。
- 完整归档位于 `%TEMP%/opsdoctor-phase09/eventstream-research/controlled/20261103T072636Z/`：`responses/` 保存 146 次响应，`unique-traces/` 保存所有对象版本，`queries.jsonl` 逐次保存 URL、时间、状态及 SHA，`artifact-manifest.json` 列出全部文件指纹。更早的 SDK 固定源和 HTTP 404/200 探测在上一级研究目录。

Jaeger 使用[有界内存存储，固定配置为最多 25000 条 trace](https://github.com/open-telemetry/opentelemetry-demo/blob/dedc0178918e260823323b8d95005a8cb924b007/compose.observability.yaml#L32)。此前已捕获的 trace 后来实际返回过 404，因此不能依靠事后一次查询恢复全部运行历史。未结束、未导出、被淘汰的流，以及未捕获的旧实例均不推断。

就这次开关周期和已捕获的四个实例而言，没有检出动作导致的新 EventStream 早终或额外短重连证据；约 600 秒时长与原有重连节奏支持上游默认 deadline 解释。单次有限观察不提供长期频率显著性结论，不证明所有部署环节、业务链路或医生取证入口都正常；已单独报告的 Jaeger 路径接入问题也不能被这项研究自动判为修复。背景数据保持原样，不因此修改医生提示、工具、手册或评分。
