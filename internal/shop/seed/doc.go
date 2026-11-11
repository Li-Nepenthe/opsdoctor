// Package seed 为 shopctl 准备实验数据。每批同时写商品、库存、订单、销量和消息去重凭据，让既有订单也能通过销量核对。造数期间须暂停下单，并避免多个 seed 同时写入。
// 从 seed.go 的 Run 读到 repository.go 的 insertBatch。完整规模是 50 万商品、每个商品 4 单，small 模式用相同规则生成 100 个商品。重跑只补完整批次之后的缺口，保留已经扣减的库存。
//
//   - seed.go：选择规模、确定续跑起点并报告已经提交的进度。
//   - repository.go：校验已有基线，组装样本并在同一事务中写入五张表。
package seed
