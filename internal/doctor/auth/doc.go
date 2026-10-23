// Package auth 医生认证的凭据规则，目前还没有接到 HTTP。
//
// 后续 doctor 接口将调用它。它依赖 JWT 与 bcrypt，不查询用户表，也不组织登录路由。
//
// 包内文件：
//   - doc.go：说明包的位置、边界和阅读路线。
//   - auth.go：医生登录要用的令牌与密码规则。
//   - auth_test.go：认证规则的边界与伪造令牌测试。
//
// 阅读顺序：auth.go → auth_test.go。
package auth
