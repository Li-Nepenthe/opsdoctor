# toxiproxy：让真实网络数据晚一点到达

应用调用下游时，数据先经过这个代理，再送到真正的服务。平时原样转发；实验时，可以只让某个方向的数据多等一会儿。业务仍执行真实 SQL 和 RPC，没有直接返回假错误。

[proxies.json](proxies.json) 固定三条路线：订单 → 库存、库存 → MySQL、订单 → Redis。`upstream` 指请求往下游发出的方向，`downstream` 指响应返回调用方的方向。延迟单位为毫秒。

Compose 固定 Toxiproxy 2.13.0，管理端口只映射到 127.0.0.1:8474，容器最多使用 128 MiB。chaos 只增删自己命名为 transport-delay 的规则，不调用会清空全部配置的 reset。这个目录不放具体场景答案。

接口与镜像依据：[官方使用说明](https://github.com/Shopify/toxiproxy#http-api)、[官方镜像](https://github.com/Shopify/toxiproxy/pkgs/container/toxiproxy)。
