// 本文件：建立医生仓储并应用医生自己的迁移。
//
// 仓储拿到的是 doctor-mysql 连接池。它不连接靶场，也没有任何 shop 表的查询。
// 数据源检查属于 source 适配器，诊断任务保存属于这里。
//
// 主要内容：
//   - Store、New：持有 app 创建的连接池，不自行创建第二套连接。
//   - Migrate：把嵌入的 SQL 交给阶段 0 的通用迁移器。
//   - newID：生成固定长度 ID，避免把任务号和数据库自增序号耦合。
//
// 阅读建议：连接池由 app 关闭；迁移器负责加锁和检查已执行文件的校验和。

package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/migrate"
)

// migrations 是编译时嵌入的只读文件，部署容器不需要读取仓库源码。
//
//go:embed migrations/*.sql
var migrations embed.FS

// Store 不向 HTTP 层暴露 sql.DB，接口不能随意拼接 SQL。
type Store struct{ database *sql.DB } // 使用医生自己的账号与数据库。

// New 只装配已有池，连接检查和生命周期由 app 统一负责。
func New(database *sql.DB) *Store { return &Store{database: database} }

// Migrate 复用命名锁和校验和，两个误启动的进程也不会同时执行同一迁移。
func Migrate(ctx context.Context, database *sql.DB) error {
	return migrate.Apply(ctx, database, migrations, "migrations")
}

// newID 用随机 16 字节编码为 32 位十六进制；ID 不包含账号或告警内容。
func newID() string {
	var bytes [16]byte
	_, _ = rand.Read(bytes[:])
	return hex.EncodeToString(bytes[:])
}
