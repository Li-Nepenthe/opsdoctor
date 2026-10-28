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
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/migrate"
)

// migrations 是编译时嵌入的只读文件，部署容器不需要读取仓库源码。
//
//go:embed migrations/*.sql
var migrations embed.FS

// Store 不向 HTTP 层暴露 sql.DB，接口不能随意拼接 SQL。
type Store struct {
	database *sql.DB // 使用医生自己的账号与数据库。
	config   Config  // 构造后固定，回调只能在事务提交之后执行。
}

// Config 把时间和通知作为依赖注入，测试无需真的等待分组过期。
type Config struct {
	RecoveryWindows map[string]time.Duration            // 按系统绑定观察期；缺少配置时拒绝接收，不能悄悄立即关闭故障。
	FaultSilence    time.Duration                       // 长时间未收到整组通知时，允许下一次 firing 开新任务。
	Redactor        *llm.Redactor                       // 入库的外部文字和发给模型的文字遵循同一套规则。
	Now             func() time.Time                    // 生产使用 time.Now，测试使用可控时间。
	Published       func(string, agent.Step)            // 步骤提交后的轻量通知，不在事务里推送网络数据。
	Changed         func(string, string, time.Duration) // 已提交的状态转换，仅用系统名和有限状态作为指标标签。
}

// New 只装配已有池，连接检查和生命周期由 app 统一负责。
func New(database *sql.DB) *Store { return NewConfigured(database, Config{}) }

// NewConfigured 保留无网络的构造过程，缺省时间与脱敏器也适用于集成测试。
func NewConfigured(database *sql.DB, config Config) *Store {
	if config.FaultSilence <= 0 {
		config.FaultSilence = 15 * time.Minute
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Redactor == nil {
		config.Redactor = llm.NewRedactor()
	}
	return &Store{database: database, config: config}
}

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
