# 仓库内部实现

Go 限制 internal 下包的外部导入；这里按通用能力与领域拆分，不放可执行程序 main 或生成协议。

目录内容：

- [platform](platform/README.md)：与业务无关的基础设施。
- [shop](shop/README.md)：故障靶场业务。
- [doctor](doctor/README.md)：医生领域，目前仅认证基础包。

阅读建议：从对应 cmd 入口进入 app，再依接口走到实现；领域边界由 archcheck 检查。
