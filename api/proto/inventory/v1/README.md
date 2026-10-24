# 库存服务接口 v1

[inventory.proto](inventory.proto) 定义了库存服务的三个方法：

| 方法 | 作用 | 常见错误 |
|---|---|---|
| GetStock | 查询某个商品当前的库存 | 商品不存在时返回 NOT_FOUND |
| Deduct | 扣减库存 | 库存不够时返回 FAILED_PRECONDITION，不会扣成负数 |
| Release | 归还库存。下单失败时，用来把已经扣掉的库存加回去 | — |

## 读的时候注意两点

**字段编号不能改。** `.proto` 里每个字段后面的数字（比如 `int64 sku_id = 1;` 里的 `1`）是这个字段在网络上传输时的身份。改了编号，旧的调用方发来的数据就会被理解成另一个字段。所以就算只是为了排版好看，也不能重新编号；删掉的字段，它的编号也不能再分配给别的字段。

**扣减和归还不能自动重试。** 调用超时时，你不知道服务端到底执行了没有。如果自动重试，可能扣两次库存。这个问题在阶段 2 的“接口幂等”里会详细讲。

实现代码在 [internal/shop/inventory](../../../../internal/shop/inventory/doc.go)，生成的 Go 代码在 [gen/inventory/v1](../../../../gen/inventory/README.md)。
