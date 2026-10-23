// Package auth 提供医生登录将要使用的 JWT 和 bcrypt 密码能力，来源于 ai-backend 的 auth 包。
// 它不访问数据库、不注册路由；阶段 5 的接入层负责查用户并调用这里的函数。
// 本包依赖 jwt/v5 和 x/crypto/bcrypt，已移除旧项目的 domain 依赖。
package auth
