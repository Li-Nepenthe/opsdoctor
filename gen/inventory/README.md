# gen/inventory：库存服务接口生成的代码

`v1` 目录里有两个文件，都是自动生成的：

| 文件 | 内容 |
|---|---|
| [inventory.pb.go](v1/inventory.pb.go) | 消息类型，比如 `DeductRequest`，以及它们和二进制格式之间的转换 |
| [inventory_grpc.pb.go](v1/inventory_grpc.pb.go) | 客户端和服务端的接口 |

学习时不用细读，只要找到这三样东西：

- **`InventoryServiceClient`**：客户端接口。订单服务和 shopctl 通过它调用库存服务。
- **`InventoryServiceServer`**：服务端接口。库存服务的 `GRPCServer` 实现了它。
- **`RegisterInventoryServiceServer`**：把服务端的实现注册到 gRPC 服务器上。
