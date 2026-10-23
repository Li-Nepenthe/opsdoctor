# Go 镜像构建

[go.Dockerfile](go.Dockerfile) 用固定 Go 镜像编译，再将纯 Go 程序放进小型非 root 运行镜像。不复制本机环境文件。

目录内容：

- 当前没有子目录。

阅读建议：先看 CMD_NAME 构建参数，再看 CGO_ENABLED=0 与多阶段 COPY；.dockerignore 位于仓库根目录。
