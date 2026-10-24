# api：服务之间的接口定义

服务之间互相调用之前，要先约定好“能调用哪些方法、每个方法传什么、返回什么”。这份约定就放在这里，用 protobuf 格式写成 `.proto` 文件。

这里只放约定本身。根据约定自动生成的 Go 代码在 [gen](../gen/README.md)，库存业务的实现在 `internal/shop/inventory`。

## 里面有什么

| 目录 | 内容 |
|---|---|
| [proto/inventory/v1](proto/inventory/v1/README.md) | 库存服务的接口：查询库存、扣减库存、归还库存 |

目前只有库存服务用 gRPC。订单服务对外提供的是 HTTP 接口，不在这里定义。

## 怎么从 .proto 生成 Go 代码

项目用 buf 这个工具来检查和生成代码，配置文件是根目录的 [buf.yaml](../buf.yaml) 和 [buf.gen.yaml](../buf.gen.yaml)。生成命令见根目录 README 的“重新生成 protobuf”一节。只有修改了 `.proto` 文件时才需要重新生成。
