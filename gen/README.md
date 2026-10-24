# gen：自动生成的代码

这里的 Go 代码全部由 buf 根据 `api/proto` 里的 `.proto` 文件自动生成，**不要手动修改**。要改接口，就去改 `.proto` 文件，再重新生成。

生成的代码也提交进了 Git。这样平时编译、跑 CI，都不需要安装 buf 和代码生成插件。

| 目录 | 内容 |
|---|---|
| [inventory](inventory/README.md) | 库存服务接口 v1 生成的代码 |
