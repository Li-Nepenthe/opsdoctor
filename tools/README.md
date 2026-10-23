# 仓库质量工具

工具由开发者和 CI 运行，不参与业务请求。规则实现与反例测试都在各 Go 包中，不放临时调试脚本。

目录内容：

- [commentcheck](commentcheck/doc.go)：检查三层说明，输出 -list 和 -map。
- [archcheck](archcheck/doc.go)：检查目录、依赖方向与答案读取。

阅读建议：先读各包 main.go，知道输入输出，再按 doc.go 路线读规则与故意违规的测试。
