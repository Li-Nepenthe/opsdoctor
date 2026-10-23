# 库存生成代码

[v1/inventory.pb.go](v1/inventory.pb.go) 保存消息编解码；[v1/inventory_grpc.pb.go](v1/inventory_grpc.pb.go) 保存客户端、服务端接口与注册。此目录不承载库存规则。

目录内容：

- v1：只放生成文件，避免手写说明随重新生成混入协议实现。

阅读建议：想改消息请回到 api/proto/inventory/v1；学习时重点找 InventoryServiceClient 与 RegisterInventoryServiceServer。
