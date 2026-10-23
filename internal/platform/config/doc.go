// Package config 把环境配置转成启动前可以信任的值。
//
// 各 app 使用 Reader；本包不选择业务默认值，也不直接创建数据库或服务器。
//
// 包内文件：
//   - doc.go：说明包的位置、边界和阅读路线。
//   - config.go：把环境变量转换为有范围约束的配置值。
//   - config_test.go：用内存环境验证配置读取和错误提示。
//
// 阅读顺序：config.go → config_test.go。
package config
