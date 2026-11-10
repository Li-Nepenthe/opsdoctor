# 报告工具字节基线

`report_definition.golden.json` 来自提交 `d092bed` 的 `reportDefinition`，保存完整工具定义，包括名称、描述和参数 Schema。

固定组件表依次为 `unknown`、`order-service`、`inventory-service`、`shop-mysql`、`shop-redis`、`kafka`。测试不读取系统配置，组件及枚举顺序不会随环境变化。

文件内容是 `json.Marshal(reportDefinition(components))` 的原始输出，没有缩进或末尾换行。测试直接比较字节。替换校验器时不得更新基线；将来有意修改格式时，应先审核对模型输入的影响。

`report_output.golden.json` 是替换生产代码前，由提交 `db0e2d4` 的旧 `validateReport` 生成的五组输入与输出。覆盖普通报告、大整数、极小指数、confidence 精度和其他数字写法。输出保存为字符串以保留完整字节，`TestReportOutputBytes` 检查双解码没有改写它们。此基线不包含当时会被旧实现拒绝的 `1.0` 步骤编号；该 bug 修复由独立回归测试覆盖。
