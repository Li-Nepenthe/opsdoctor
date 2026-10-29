> 正式评测前的完整检查，评分接线未通过。五份医生报告和费用均保留，不能纳入正式矩阵。

# 评测记录 phase08-lastcheck3-20261029

这张表来自自建靶场。请按批次配置区分试跑和正式评测；每个场景样本很少，不能代表生产准确率。OpenAI 金额是用量账本，实际扣费由用户在平台核对。

批次 token：344002 / 10000000（含未确认预留）。

| 场景 | 轮次 | 组 | 诊断 | 状态 | 组件一致 | 轮数 | 工具 | 输入 / 输出 token | 账本费用 | 耗时秒 | 提交方式 | 混合模型 |
|---|---:|---|---|---|---|---:|---:|---:|---:|---:|---|---|
| S01 | 1 | E2 | c4e699b8e96e43fa7be8206e5aeee1bc | succeeded | true | 4 | 15 | 41057 / 1149 | USD 0.023853 | 23.911 | voluntary | false |
| S01 | 1 | E1 | 9a50497b0ab6ead9cb1d812cd7e167e5 | succeeded | true | 13 | 22 | 199986 / 6701 | CNY 0.053186 | 44.046 | voluntary | false |
| S01 | 1 | E0 | 1aad4f51f4899bf321b4ac14fd1fba1c | succeeded | false | 1 | 0 | 3781 / 278 | USD 0.006116 | 4.570 | voluntary | false |
| S01 | 1 | E1 | d5f2486fbdbb643926c389d71514e9e1 | succeeded | true | 4 | 15 | 43596 / 1110 | USD 0.024173 | 22.730 | voluntary | false |
| S01 | 1 | E3 | c8c81fb5bc3f331dcb06502bb3ef2eef | succeeded | true | 5 | 18 | 45199 / 1145 | USD 0.022298 | 23.525 | voluntary | false |

## 分层结果

每格为平均值 [最小, 最大] n=有效样本数。组件一致记 1，否则 0；机制正确/部分/错误记 1/0.5/0。基础设施失败和混合模型单列；评分失败不填零。三次样本很小，以下不是生产准确率。

| 场景 | 组 | 模型 | 组件一致 | 机制 | 证据覆盖 | 轮数 | 工具数 | 秒 | 医生单次账本 | 全部医生费用（含排除项） | 评委费用 | 可靠性 / 混合 / 未完成 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| S01 | E0 | gpt-6.1-terra | 0.000 [0.000, 0.000] n=1 | 未测量 | 未测量 | 1.000 [1.000, 1.000] n=1 | 0.000 [0.000, 0.000] n=1 | 4.570 [4.570, 4.570] n=1 | USD 0.006 [0.006, 0.006] n=1 | USD 0.006116 | CNY 0.000000 | 0 / 0 / 0 |
| S01 | E1 | deepseek-flash | 1.000 [1.000, 1.000] n=1 | 未测量 | 未测量 | 13.000 [13.000, 13.000] n=1 | 22.000 [22.000, 22.000] n=1 | 44.046 [44.046, 44.046] n=1 | CNY 0.053 [0.053, 0.053] n=1 | CNY 0.053186 | CNY 0.000000 | 0 / 0 / 0 |
| S01 | E1 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=1 | 未测量 | 未测量 | 4.000 [4.000, 4.000] n=1 | 15.000 [15.000, 15.000] n=1 | 22.730 [22.730, 22.730] n=1 | USD 0.024 [0.024, 0.024] n=1 | USD 0.024173 | CNY 0.000000 | 0 / 0 / 0 |
| S01 | E2 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=1 | 未测量 | 未测量 | 4.000 [4.000, 4.000] n=1 | 15.000 [15.000, 15.000] n=1 | 23.911 [23.911, 23.911] n=1 | USD 0.024 [0.024, 0.024] n=1 | USD 0.023853 | CNY 0.000000 | 0 / 0 / 0 |
| S01 | E3 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=1 | 未测量 | 未测量 | 5.000 [5.000, 5.000] n=1 | 18.000 [18.000, 18.000] n=1 | 23.525 [23.525, 23.525] n=1 | USD 0.022 [0.022, 0.022] n=1 | USD 0.022298 | CNY 0.000000 | 0 / 0 / 0 |

## 评分与失败明细

以下是原始结论与评委理由；评委也是模型，人工一致率未填写前不能视为标准答案。格式、预算、可靠性和评分失败分别显示。推理错误、工具不足、数据缺失以及评分有误的最终分类，需要结合原步骤逐条核对。

| 槽位 | 医生错误 / 预算原因 | 机制 / 评分状态 | 评分理由 | 证据覆盖 | 评委账本 | S12 实际暴露 | 写提议 / 执行 |
|---|---|---|---|---|---|---|---|
| phase08-lastcheck3-20261029/S01/1/primary-E2 |   | 评分失败 | 编码模型请求：json: error calling MarshalJSON for type *jsontext.Value: unexpected end of JSON input | — | CNY 0.000000 | false | 1 / 0 |
| phase08-lastcheck3-20261029/S01/1/comparison-E1 |   | 评分失败 | 编码模型请求：json: error calling MarshalJSON for type *jsontext.Value: unexpected end of JSON input | — | CNY 0.000000 | false | 0 / 0 |
| phase08-lastcheck3-20261029/S01/1/primary-E0 |   | 评分失败 | 编码模型请求：json: error calling MarshalJSON for type *jsontext.Value: unexpected end of JSON input | — | CNY 0.000000 | false | 0 / 0 |
| phase08-lastcheck3-20261029/S01/1/primary-E1 |   | 评分失败 | 编码模型请求：json: error calling MarshalJSON for type *jsontext.Value: unexpected end of JSON input | — | CNY 0.000000 | false | 1 / 0 |
| phase08-lastcheck3-20261029/S01/1/primary-E3 |   | 评分失败 | 编码模型请求：json: error calling MarshalJSON for type *jsontext.Value: unexpected end of JSON input | — | CNY 0.000000 | false | 1 / 0 |
