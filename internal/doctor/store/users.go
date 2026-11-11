// 本文件：按标准化邮箱保存或查找登录账号。仓储只接收密码摘要，重复启动时保留已有密码。

package store

import "context"

// User 只在服务器内部用于验证登录，不能把整个结构体编码成 HTTP 响应。
type User struct {
	ID           string // 登录成功后放进 JWT 的用户标识。
	Email        string // 已经标准化的邮箱。
	PasswordHash string // bcrypt 摘要，不发给客户端。
	Status       string // 只有 active 用户可以登录。
}

// EnsureUser 使用唯一邮箱处理重复启动，不把初始化配置当成重置密码功能。
func (store *Store) EnsureUser(ctx context.Context, email, hash string) error {
	_, err := store.database.ExecContext(ctx, "INSERT INTO users(id,email,password_hash) VALUES(?,?,?) ON DUPLICATE KEY UPDATE id=id", newID(), email, hash)
	return err
}

// UserByEmail 不区分大小写的规则由 auth 统一处理，这里不再改写输入。
func (store *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := store.database.QueryRowContext(ctx, "SELECT id,email,password_hash,status FROM users WHERE email=?", email).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Status)
	return user, err
}
