# 阶段 9 第二次试跑：评委 JSON 失败定位

2026-11-03。批次 `phase09-pilot2-20261103-terra` 的医生已正常提交，采集质量为 normal；DeepSeek 的 API 请求成功，但返回的评分工具参数不是合法 JSON，冻结校验拒绝了这份评分。正式批次未开启。本次只读调查没有修改记录、修补回答、重新评分或再次调用模型。

## 1. 保留的原始记录

本次读取独立状态库 `opsdoctor-phase09-doctor-mysql`，只对固定 judge ID `ff2899694a53ceec2e2f7e6448918fac` 和 attempt ID `154950629781b2aa9f425cfc0c7f9fa9` 执行 UTF8 SELECT，没有枚举容器环境或读取模型 Key。

已脱敏的 API 账本与评分原文分别保存在[请求](phase-09-pilot2-judge-request.log)、[响应](phase-09-pilot2-judge-response.log)、[已存评分](phase-09-pilot2-judge-judgement.log)及[关联记录](phase-09-pilot2-judge-record.log)。这些是数据库 JSON 列读出的原始文本，保留 MySQL 输出的换行，经二进制复制并计算 SHA256；不声称它们是未脱敏的网络抓包。`.log` 已由仓库现有 `.gitattributes` 禁止换行转换。完整指纹、原 Temp 路径及定位结果见[量化 JSON](phase-09-pilot2-judge-failure.json)。

## 2. 失败发生在哪一层

外层 API 响应本身是合法 JSON，模型为 `deepseek-flash`，恰好返回一次 `submit_grade`，符合 [CheckResponse](../../internal/eval/judge/grade.go) 对工具名与调用次数的要求。

失败字段是 `choices[0].message.tool_calls[0].function.arguments`。这段字符串共 747 个 UTF8 字节，其中第一项里程碑的 `reason` 文本没有在逗号前闭合引号。错误附近的连续原文为：

```text
同样显示错误。, "steps": "报告主因与参考一致
```

因此，`steps` 前面的引号被解释为结束 `reason` 字符串；接下来的 `s` 出现在应当是逗号或右花括号的位置。只做语法检查的 Python JSON 解析器在第 243 个 Unicode 字符、第 435 个 UTF8 字节处报 `Expecting ',' delimiter`，两处位置均从 1 开始计数。完整的[参数字符串](phase-09-pilot2-judge-arguments.log)从原响应解码得到，内容没有修补；它是便于查看的派生文件，原外层响应另行保留。

冻结的 `Check` 在第一道格式检查中执行 `decoder.Decode(&wire)`，此时已经失败，于是返回 `评分 JSON 格式不合法`。数据库中的 judge 错误和 stored judgement 错误与它完全一致。程序没有进入机制采纳、里程碑完整性、步骤引用或覆盖率计算。

## 3. 冻结协议是否确实送到评委

请求中的系统评分标准、当前冻结 `rubric.md` 和 stored judgement 的评分标准指纹完全一致：

```text
bf5252244c3959a6d5d0396aa229c426f9a8de71aabdf6cdb6802079edc4aa9f
```

版本均为 `rubric-v1.1`。请求明确列出待评编号 1、2、3，规则项为空；工具 Schema 要求恰好三项，`steps` 为整数数组。相关生成与校验代码见 [protocol.go](../../internal/eval/judge/protocol.go) 和 [grade.go](../../internal/eval/judge/grade.go)。这次已经在 JSON 结构解析时失败，不把后续未执行的检查说成另一次实测判定。

响应的 `finish_reason` 为 `tool_calls`，输出为 466 token，请求上限为 2048 token。这些记录没有证明回答因输出 token 上限而截断，也不足以定位供应商内部的生成原因。

## 4. 记录与费用如何解释

医生摘要为 `succeeded`、`report_mode=voluntary`、`data_quality=normal`，未混用模型，也未标记可靠性失败；输入 50,506、输出 1,323 token，美元项目账本费用为 **$0.027179**。

美元按项目用量账本计；Key 与其他用途共用，平台账单无法单独核对。人民币与美元分别记录，不相加。

指定评委 attempt 已结算，API 账本状态为 `ok`；输入 13,808、输出 466 token，人民币项目账本费用为 **¥0.014920**。随后应用层校验失败，judge 诊断与 stored judgement 均为 `failed`。API 请求成功与评分协议成功是分别记录的状态，不冲销已经发生的费用。

参数字符串中虽然出现字面量 `mechanism=correct`，它没有成为有效评分。stored judgement 的机制字段为空；其 `coverage=0` 是失败记录的默认值，不能当作实测零覆盖率或填入正确率。医生报告继续保留，评委失败继续单列；没有改判或补一次评分。

## 5. 当前停点

用户要求的正式批次门槛包括“评委成功”，本次没有满足。因此即使其他检查或费用预测通过，也不能进入正式批次，更不能为了满足这项门槛而追加一轮付费评分。

原诊断、原 API 账本、已存失败评分及其费用均保留。故障开关恢复、健康检查、总账及主文档由本轮主任务另行归档；本报告只定位这份评委返回的协议失败，不提出或应用新的评分版本。
