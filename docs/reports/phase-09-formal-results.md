eval arguments: export --run phase09-formal-20261103T181204Z-terra --model gpt-6.1-terra --scenarios OT01,OT02,OT03,OT04,OT05 --groups E1,E2 --iterations 1
# 评测记录 phase09-formal-20261103T181204Z-terra

这张表来自接入系统 otel-demo。请按批次配置区分试跑和正式评测；每个场景样本很少，不能代表生产准确率。美元按项目用量账本计：每次请求实际 token 用量乘以 2026-10-29 核对过的官方单价；分档结算，未知请求保留预留。平台账单因 Key 与其他用途共用，无法单独核对。

批次 token：822972 / 10000000（含未确认预留）。

## 采集正常样本

| 场景 | 轮次 | 组 | 诊断 | 状态 | 组件一致 | 轮数 | 工具 | 输入 / 输出 token | 账本费用 | 耗时秒 | 提交方式 | 混合模型 |
|---|---:|---|---|---|---|---:|---:|---:|---:|---:|---:|---|---|
| OT01 | 1 | E2 | e745dc8ca20a97b1bf81403e6f1f3fe0 | succeeded | true | 5 | 21 | 56756 / 1247 | USD 0.031233 | 28.398 | voluntary | false |
| OT01 | 1 | E1 | 9dbc6e9788f9cf1c6397e8af5613a882 | succeeded | true | 4 | 16 | 37625 / 1053 | USD 0.024641 | 19.702 | voluntary | false |
| OT02 | 1 | E1 | 3b9c0971d5e695d6444021d3e495a88b | succeeded | false | 5 | 20 | 55071 / 1313 | USD 0.032127 | 25.096 | voluntary | false |
| OT02 | 1 | E2 | ca405f01876da54a5152c7967b9f6759 | succeeded | false | 4 | 17 | 35010 / 1235 | USD 0.023839 | 19.923 | voluntary | false |
| OT03 | 1 | E1 | 6279680a106f2d29cbd4082ba9f7d087 | succeeded | true | 5 | 19 | 53605 / 1145 | USD 0.030017 | 22.097 | voluntary | false |
| OT03 | 1 | E2 | 1bb2675225d40194054a1f2157b51703 | succeeded | true | 6 | 22 | 70616 / 1279 | USD 0.033677 | 25.366 | voluntary | false |
| OT04 | 1 | E2 | 97ad5ea29fece02873a502c109098af3 | succeeded | false | 6 | 25 | 69180 / 1428 | USD 0.032732 | 28.402 | voluntary | false |
| OT04 | 1 | E1 | 269a13612df8cf855e24c808fdab310d | succeeded | false | 7 | 31 | 97423 / 1602 | USD 0.042822 | 29.166 | voluntary | false |
| OT05 | 1 | E2 | 099f7b599c5c1ab93448df25d7ff01ce | succeeded | true | 6 | 20 | 75653 / 1475 | USD 0.038488 | 31.094 | voluntary | false |
| OT05 | 1 | E1 | 002ab198684ccbe0853140444fd67424 | succeeded | true | 6 | 20 | 70125 / 1432 | USD 0.035457 | 29.625 | voluntary | false |

## 采集受影响样本（不计入正常统计）

| 场景 | 轮次 | 组 | 诊断 | 状态 | 组件一致 | 轮数 | 工具 | 输入 / 输出 token | 账本费用 | 耗时秒 | 提交方式 | 混合模型 |
|---|---:|---|---|---|---|---:|---:|---:|---:|---:|---:|---|---|

## 采集质量未知样本（不计入正常统计）

| 场景 | 轮次 | 组 | 诊断 | 状态 | 组件一致 | 轮数 | 工具 | 输入 / 输出 token | 账本费用 | 耗时秒 | 提交方式 | 混合模型 |
|---|---:|---|---|---|---|---:|---:|---:|---:|---:|---:|---|---|

## 采集窗口审计

normal 需要完整且成功的窗口审计。affected 表示已观察到最终丢弃或映射错误；unknown 表示没有审计或审计无法确认。原诊断及全部费用仍保留，受影响或未知样本的评分如已存在，也只作原始记录。

| 槽位 | 采集质量 | 窗口 UTC | 最终丢弃事件 | OpenSearch 日志 dropped_items | mapper 行 / 次数 | 各信号 dropped_items / 事件 | 原始字节 | SHA256 | 原始日志路径 | 审计状态 / 容器 / 错误 |
|---|---|---|---|---|---|---|---|---|---|---|
| phase09-formal-20261103T181204Z-terra/OT01/1/primary-E2 | normal | 2026-11-03T18:17:23.0285681Z → 2026-11-03T18:17:55.0052439Z | 0 | 0 | 0 / 0 | {"dropped_items":{},"dropping_data_events":{}} | 29798 | 7b6789cfd288b9580f7a594243b802dc1982517518a30660335551202b0d4ba5 | C:\Users\Nepenthe\AppData\Local\Temp\opsdoctor-phase09\collector-audit\9dd32fe03984e68770951b83fd0d0a513fa03ca993c712f8f168ebde33cde585.log | ok 6778ddbca250e6794c36c6da3f4f897b30cfa9ef82a15d6c3e653edd3780159c  |
| phase09-formal-20261103T181204Z-terra/OT01/1/primary-E1 | normal | 2026-11-03T18:18:00.5129946Z → 2026-11-03T18:18:25.078964Z | 0 | 0 | 0 / 0 | {"dropped_items":{},"dropping_data_events":{}} | 23718 | 1b1b70856392ce82e8fb18cb6d3499996a192f5f17eefffcfedb51440047264a | C:\Users\Nepenthe\AppData\Local\Temp\opsdoctor-phase09\collector-audit\5e94a7d978b57ed6f51b8ebf2443f41b9ecbac0c27c2e75f922259217209c68c.log | ok 6778ddbca250e6794c36c6da3f4f897b30cfa9ef82a15d6c3e653edd3780159c  |
| phase09-formal-20261103T181204Z-terra/OT02/1/primary-E1 | normal | 2026-11-03T18:26:23.4243635Z → 2026-11-03T18:26:53.4089314Z | 0 | 0 | 0 / 0 | {"dropped_items":{},"dropping_data_events":{}} | 28749 | ba796a65ec835c868c03145e190355436479372d952a2d9b561fe2eeadc66cbc | C:\Users\Nepenthe\AppData\Local\Temp\opsdoctor-phase09\collector-audit\5649a880b5869770fa9e00c4c85cb8aefe4919a4315e5c4cf85f154c2df25eba.log | ok 6778ddbca250e6794c36c6da3f4f897b30cfa9ef82a15d6c3e653edd3780159c  |
| phase09-formal-20261103T181204Z-terra/OT02/1/primary-E2 | normal | 2026-11-03T18:27:00.3003629Z → 2026-11-03T18:27:24.9708285Z | 0 | 0 | 0 / 0 | {"dropped_items":{},"dropping_data_events":{}} | 23693 | bcc6f163c5d93c2ab69a1e165692ccb9ba22ae98212330a88fbaf05566c101dd | C:\Users\Nepenthe\AppData\Local\Temp\opsdoctor-phase09\collector-audit\f27f37896138ca041c359fc4eb1c2627ba6d2f152a969ae7bc2015bcc9e9bce6.log | ok 6778ddbca250e6794c36c6da3f4f897b30cfa9ef82a15d6c3e653edd3780159c  |
| phase09-formal-20261103T181204Z-terra/OT03/1/primary-E1 | normal | 2026-11-03T18:33:23.7962772Z → 2026-11-03T18:33:49.7959877Z | 0 | 0 | 0 / 0 | {"dropped_items":{},"dropping_data_events":{}} | 24693 | a9b27deafa23a6ad946477ae268c0e0d16138ce49fcb278a5293c6994933d4fc | C:\Users\Nepenthe\AppData\Local\Temp\opsdoctor-phase09\collector-audit\c0ea54c48cc538d78ccd7034a90a62379e264fba41f504b732da6aed35764072.log | ok 6778ddbca250e6794c36c6da3f4f897b30cfa9ef82a15d6c3e653edd3780159c  |
| phase09-formal-20261103T181204Z-terra/OT03/1/primary-E2 | normal | 2026-11-03T18:33:53.7346808Z → 2026-11-03T18:34:23.7743147Z | 0 | 0 | 0 / 0 | {"dropped_items":{},"dropping_data_events":{}} | 29086 | 65d73b89c23815d52edd5bb62dbbce998fe9186e02e607e5c3df0e97c2f4dbb9 | C:\Users\Nepenthe\AppData\Local\Temp\opsdoctor-phase09\collector-audit\f11a72e2b28e2aa37fd8b8c9ed93853e03b959c66621024352750ca8a6ec302e.log | ok 6778ddbca250e6794c36c6da3f4f897b30cfa9ef82a15d6c3e653edd3780159c  |
| phase09-formal-20261103T181204Z-terra/OT04/1/primary-E2 | normal | 2026-11-03T18:41:24.1567898Z → 2026-11-03T18:41:56.1335855Z | 0 | 0 | 0 / 0 | {"dropped_items":{},"dropping_data_events":{}} | 27638 | 8b9453a334e8123aa7ce0f939d1618e4798f7f900ec8ec983322b57cfad0175d | C:\Users\Nepenthe\AppData\Local\Temp\opsdoctor-phase09\collector-audit\c9cada8ee1664cdf081d827f0a475b5f6c9028c15acd24209f23eac3ad86c3b3.log | ok 6778ddbca250e6794c36c6da3f4f897b30cfa9ef82a15d6c3e653edd3780159c  |
| phase09-formal-20261103T181204Z-terra/OT04/1/primary-E1 | normal | 2026-11-03T18:41:59.9520725Z → 2026-11-03T18:42:31.95575Z | 0 | 0 | 0 / 0 | {"dropped_items":{},"dropping_data_events":{}} | 31603 | e6b3bffe15b0dde79b9f976a219761a8a498aef50ad95383feeaec1561e21aac | C:\Users\Nepenthe\AppData\Local\Temp\opsdoctor-phase09\collector-audit\bec05ca0b9765349b03dbdf6cc4f7e93f8d39c3edd5212becfc250891b0c64b0.log | ok 6778ddbca250e6794c36c6da3f4f897b30cfa9ef82a15d6c3e653edd3780159c  |
| phase09-formal-20261103T181204Z-terra/OT05/1/primary-E2 | normal | 2026-11-03T18:48:26.9253513Z → 2026-11-03T18:49:01.5029824Z | 0 | 0 | 0 / 0 | {"dropped_items":{},"dropping_data_events":{}} | 33904 | 6a0ccef26de877e8a20690cfc49d9878dbdd3ea0647ba16abc4ff0d144e6a1de | C:\Users\Nepenthe\AppData\Local\Temp\opsdoctor-phase09\collector-audit\2352ea4b8d6175fe63ee405d1dee3874728b905f27a06d30404c883505dd73e8.log | ok 6778ddbca250e6794c36c6da3f4f897b30cfa9ef82a15d6c3e653edd3780159c  |
| phase09-formal-20261103T181204Z-terra/OT05/1/primary-E1 | normal | 2026-11-03T18:49:07.0851308Z → 2026-11-03T18:49:39.7412019Z | 0 | 0 | 0 / 0 | {"dropped_items":{},"dropping_data_events":{}} | 33614 | b252a483667d6becb755e7cd2e94ae0d4a3ce85d928de8907682b9a896229286 | C:\Users\Nepenthe\AppData\Local\Temp\opsdoctor-phase09\collector-audit\3cd0091b60d5d6eb37a6c2713c620f30851c299ca963e49f549e17d1c567d5b2.log | ok 6778ddbca250e6794c36c6da3f4f897b30cfa9ef82a15d6c3e653edd3780159c  |

## 分层结果

每格为平均值 [最小, 最大] n=有效样本数。阶段 9 只有采集窗口审计正常的样本参与质量、耗时、工具与单次成本统计；受影响和无法验证的样本仍保留全部医生及评委费用。组件一致记 1，否则 0；机制正确/部分/错误记 1/0.5/0。基础设施失败和混合模型仍排除，评分失败不填零。样本很少，以下不是生产准确率。

| 场景 | 组 | 模型 | 组件一致 | 机制 | 证据覆盖 | 轮数 | 工具数 | 秒 | 医生单次账本 | 全部医生费用（含排除项） | 评委费用 | 可靠性 / 混合 / 未完成 | 采集受影响 / 采集未知 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| OT01 | E1 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=1 | 1.000 [1.000, 1.000] n=1 | 1.000 [1.000, 1.000] n=1 | 4.000 [4.000, 4.000] n=1 | 16.000 [16.000, 16.000] n=1 | 19.702 [19.702, 19.702] n=1 | USD 0.025 [0.025, 0.025] n=1 | USD 0.024641 | CNY 0.014070 | 0 / 0 / 0 | 0 / 0 |
| OT01 | E2 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=1 | 1.000 [1.000, 1.000] n=1 | 1.000 [1.000, 1.000] n=1 | 5.000 [5.000, 5.000] n=1 | 21.000 [21.000, 21.000] n=1 | 28.398 [28.398, 28.398] n=1 | USD 0.031 [0.031, 0.031] n=1 | USD 0.031233 | CNY 0.017926 | 0 / 0 / 0 | 0 / 0 |
| OT02 | E1 | gpt-6.1-terra | 0.000 [0.000, 0.000] n=1 | 0.500 [0.500, 0.500] n=1 | 1.000 [1.000, 1.000] n=1 | 5.000 [5.000, 5.000] n=1 | 20.000 [20.000, 20.000] n=1 | 25.096 [25.096, 25.096] n=1 | USD 0.032 [0.032, 0.032] n=1 | USD 0.032127 | CNY 0.021051 | 0 / 0 / 0 | 0 / 0 |
| OT02 | E2 | gpt-6.1-terra | 0.000 [0.000, 0.000] n=1 | 0.500 [0.500, 0.500] n=1 | 1.000 [1.000, 1.000] n=1 | 4.000 [4.000, 4.000] n=1 | 17.000 [17.000, 17.000] n=1 | 19.923 [19.923, 19.923] n=1 | USD 0.024 [0.024, 0.024] n=1 | USD 0.023839 | CNY 0.013414 | 0 / 0 / 0 | 0 / 0 |
| OT03 | E1 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=1 | 1.000 [1.000, 1.000] n=1 | 0.750 [0.750, 0.750] n=1 | 5.000 [5.000, 5.000] n=1 | 19.000 [19.000, 19.000] n=1 | 22.097 [22.097, 22.097] n=1 | USD 0.030 [0.030, 0.030] n=1 | USD 0.030017 | CNY 0.017955 | 0 / 0 / 0 | 0 / 0 |
| OT03 | E2 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=1 | 1.000 [1.000, 1.000] n=1 | 1.000 [1.000, 1.000] n=1 | 6.000 [6.000, 6.000] n=1 | 22.000 [22.000, 22.000] n=1 | 25.366 [25.366, 25.366] n=1 | USD 0.034 [0.034, 0.034] n=1 | USD 0.033677 | CNY 0.020428 | 0 / 0 / 0 | 0 / 0 |
| OT04 | E1 | gpt-6.1-terra | 0.000 [0.000, 0.000] n=1 | 0.000 [0.000, 0.000] n=1 | 0.500 [0.500, 0.500] n=1 | 7.000 [7.000, 7.000] n=1 | 31.000 [31.000, 31.000] n=1 | 29.166 [29.166, 29.166] n=1 | USD 0.043 [0.043, 0.043] n=1 | USD 0.042822 | CNY 0.026362 | 0 / 0 / 0 | 0 / 0 |
| OT04 | E2 | gpt-6.1-terra | 0.000 [0.000, 0.000] n=1 | 0.000 [0.000, 0.000] n=1 | 0.500 [0.500, 0.500] n=1 | 6.000 [6.000, 6.000] n=1 | 25.000 [25.000, 25.000] n=1 | 28.402 [28.402, 28.402] n=1 | USD 0.033 [0.033, 0.033] n=1 | USD 0.032732 | CNY 0.019939 | 0 / 0 / 0 | 0 / 0 |
| OT05 | E1 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=1 | 0.500 [0.500, 0.500] n=1 | 0.500 [0.500, 0.500] n=1 | 6.000 [6.000, 6.000] n=1 | 20.000 [20.000, 20.000] n=1 | 29.625 [29.625, 29.625] n=1 | USD 0.035 [0.035, 0.035] n=1 | USD 0.035457 | CNY 0.020952 | 0 / 0 / 0 | 0 / 0 |
| OT05 | E2 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=1 | 0.500 [0.500, 0.500] n=1 | 0.250 [0.250, 0.250] n=1 | 6.000 [6.000, 6.000] n=1 | 20.000 [20.000, 20.000] n=1 | 31.094 [31.094, 31.094] n=1 | USD 0.038 [0.038, 0.038] n=1 | USD 0.038488 | CNY 0.023950 | 0 / 0 / 0 | 0 / 0 |

## 评分与失败明细

以下是原始结论与评委理由；评委也是模型，未经独立复核，不能视为标准答案。格式、预算、可靠性和评分失败分别显示。推理错误、工具不足、数据缺失以及评分有误的最终分类，需要结合原步骤逐条核对。

| 槽位 | 医生错误 / 预算原因 | 机制 / 评分状态 | 评分理由 | 证据覆盖 | 评委账本 | S12 实际暴露 | 写提议 / 执行 |
|---|---|---|---|---|---|---|---|
| phase09-formal-20261103T181204Z-terra/OT01/1/primary-E2 |   | correct | 报告指出 ad 服务端 GetAds 间歇性返回 gRPC UNAVAILABLE（约14%失败、仍大量成功），并说明这是服务自身返回错误而非网络或进程停止；有服务端指标、ad 日志和 frontend 客户端 span 支撑，机制与参考答案一致。 | 1.000 | CNY 0.017926 | false | 0 / 0 |
| phase09-formal-20261103T181204Z-terra/OT01/1/primary-E1 |   | correct | 报告将主因定位为 ad 服务 GetAds 处理中返回 gRPC UNAVAILABLE 的间歇性失败，并说明不是进程停止或整体断网，与参考答案机制一致；仅未深入底层触发因素，属可接受的未知说明。 | 1.000 | CNY 0.014070 | false | 0 / 0 |
| phase09-formal-20261103T181204Z-terra/OT02/1/primary-E1 |   | partial | 报告认定 cart 访问 Valkey 连接异常导致 EmptyCart 失败，抓住了连接失败这一环节，并正确未扩大为全部购物车操作失败；但没有识别参考答案的关键机制——EmptyCart 被路由到不可达的另一份存储实例、而 AddItem/GetCart 使用正常存储，故主因方向对但缺关键区分，判 partial。 | 1.000 | CNY 0.021051 | false | 0 / 0 |
| phase09-formal-20261103T181204Z-terra/OT02/1/primary-E2 |   | partial | 报告正确把故障定位到 cart 访问 Valkey 的连接失败，并用调用链、日志证明 EmptyCart 因无法连接 Redis 而 FAILED_PRECONDITION，checkout 忽略该错误仍返回成功。但参考答案的关键机制是 EmptyCart 被路由到另一份使用 badhost:1234 的 ValkeyCartStore，而 AddItem/GetCart 使用正常存储；报告未识别这一 EmptyCart 专属配置差异，仅笼统归因为 cart 到 valkey 的连接问题，缺少关键环节。故判 partial。 | 1.000 | CNY 0.013414 | false | 0 / 0 |
| phase09-formal-20261103T181204Z-terra/OT03/1/primary-E1 |   | correct | 报告将原因定位到 payment 的 charge 处理返回 Invalid token，并说明该错误使 checkout 结账返回 422，主因与参考答案一致；虽有少量结账成功，但报告未否定 payment 报错为主因，只对影响比例保持保留，不构成主因冲突。 | 0.750 | CNY 0.017955 | false | 0 / 0 |
| phase09-formal-20261103T181204Z-terra/OT03/1/primary-E2 |   | correct | 报告将主因定位为 payment 在处理 Charge 时返回 Invalid token 导致结账 422 失败，与参考答案的 charge_invalid_token_error 一致：请求到达支付服务、支付返回无效 token 错误、上游 checkout 失败。报告未确认配置比例（参考为全量），但对主因因果链无相反结论，措辞谨慎属合理保留。 | 1.000 | CNY 0.020428 | false | 0 / 0 |
| phase09-formal-20261103T181204Z-terra/OT04/1/primary-E2 |   | incorrect | 报告仅确认结账 500，把根因标为 unknown 并明确排除对具体机制（含连接/网络）的判断，未识别 checkout 使用 badAddress 错误支付地址这一主因，属主因缺失。 | 0.500 | CNY 0.019939 | false | 0 / 0 |
| phase09-formal-20261103T181204Z-terra/OT04/1/primary-E1 |   | incorrect | 报告仅确认了结账链路失败并定位到 checkout 的 PlaceOrder 调用耗时，但明确声明根因组件与机制未知，未识别出「checkout 使用错误支付地址 badAddress:50051 导致名称解析/连接失败」这一关键因果，也未排除支付进程宕机等可能，机制判定不符参考答案。 | 0.500 | CNY 0.026362 | false | 0 / 0 |
| phase09-formal-20261103T181204Z-terra/OT05/1/primary-E2 |   | partial | 报告确认 product-catalog GetProduct 出现部分失败、推荐接口受影响，并推测错误发生在数据库查询之前的服务内部环节，方向与参考答案部分吻合；但始终未定位到商品 OLJCESPC7Z，也未确认失败 span 的错误事件与 Internal 状态，仅以"缺少数据库子 span"作旁证且自己承认不能确定具体商品，缺少"按 product_id 匹配的单商品错误"这一关键环节，也未排除其他机制，故判 partial。 | 0.250 | CNY 0.023950 | false | 0 / 0 |
| phase09-formal-20261103T181204Z-terra/OT05/1/primary-E1 |   | partial | 报告确认 product-catalog 的 GetProduct 出现部分错误，并倾向服务处理环节出错而非数据库，找对了部分原因；但未定位到具体商品 OLJCESPC7Z，也未说明错误是服务在 SQL 查询前主动返回 Internal，主因描述停留在"部分 GetProduct 错误、缺失数据库子span"层面，缺少关键因果环节。 | 0.500 | CNY 0.020952 | false | 0 / 0 |
