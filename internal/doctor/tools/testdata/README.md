# 工具定义字节基线

`tool_definitions.golden.json` 来自提交 `d092bed` 的现有注册表，包含十六个取证工具和可选的 `runbook_search`，按 `Registry.List()` 的顺序排列。只登记假数据源，不执行工具，也不读取真实手册。

文件内容是 `json.Marshal(registry.List())` 的原始输出，没有缩进或末尾换行。`TestToolDefinitionsGolden` 直接比较字节，字段顺序、描述、默认值和零值省略都在检查范围内。

替换校验器时不得更新此文件来消除测试失败。如果以后确实要调整工具定义，应先审阅字节差异和模型输入变化，再单独更新基线。
