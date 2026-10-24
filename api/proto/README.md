# api/proto：protobuf 文件

buf 把这个目录当作所有 `.proto` 文件的根。目录路径和 `.proto` 里的 `package` 名字一一对应：`inventory/v1/` 下的文件，`package` 就是 `inventory.v1`。

| 目录 | 内容 |
|---|---|
| [inventory](inventory/README.md) | 库存服务的接口，按版本分目录 |

从 `inventory/v1/inventory.proto` 开始读。
