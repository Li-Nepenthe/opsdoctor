// Package config 在服务连接数据库、开放端口之前检查环境变量。各服务和命令自行决定配置名、默认值和允许范围。
//
//   - doc.go 说明配置校验与服务组装的分工。
//   - config.go 实现 Reader，从 String 开始读可以看到各类型共用的读取规则。
package config
