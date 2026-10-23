// Package seed 建立靶场的商品、库存与历史订单基线。
//
// shopctl 调用 Run；每批五表一起提交，SQL 在 repository.go。这里不发送 Kafka 消息，不执行真实下单；造数期间不要同时业务写入或并发造数。
//
// 包内文件：
//   - doc.go：说明包的位置、边界和阅读路线。
//   - repository.go：批量造数的五表参数和事务写入。
//   - seed.go：控制完整规模与 small 模式的造数进度。
//   - seed_integration_test.go：用独立 MySQL 库验证续跑、跳过与回滚。
//   - seed_test.go：在内存中核对造数行数、编号和占位符。
//
// 阅读顺序：seed.go → repository.go → seed_test.go → seed_integration_test.go。
package seed
