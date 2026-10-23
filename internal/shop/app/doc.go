// Package app 保存 shop 程序共同的数据库配置读取步骤。
//
// inventoryservice 和 shopctl 子包调用 DatabaseConfig；本包依赖 config/mysqlx，不创建资源，不放库存规则。
//
// 包内文件：
//   - doc.go：说明包的位置、边界和阅读路线。
//   - config.go：读取各 shop 程序共用的数据库配置。
//   - config_test.go：验证连接池默认值和参数之间的约束。
//
// 阅读顺序：config.go → config_test.go，再进入各子包。
package app
