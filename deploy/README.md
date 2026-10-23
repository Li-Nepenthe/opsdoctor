# 本机部署

Compose 负责基础设施与已实现服务，配置文件按组件归类。不放 Go 业务实现，也不保存真实密码。

目录内容：

- [docker](docker/README.md)：所有 Go 服务共用的构建文件。
- [mysql](mysql/README.md)：首次建库与测试权限。

阅读建议：从 [docker-compose.yml](docker-compose.yml) 看服务依赖、端口和健康检查；密码从根目录被忽略的 .env 注入。
