# MySQL 初始化

这里只管理建库与权限，业务表结构由 internal/shop/migrations 管理，避免两套迁移相互覆盖。

目录内容：

- [init](init/README.md)：官方镜像首次启动时读取的 SQL。

阅读建议：旧数据卷不会重跑 init，升级测试库时按根 README 手动执行新增脚本；不要为此删卷。
