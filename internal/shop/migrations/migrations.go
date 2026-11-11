// 本文件：将 shop 的迁移随二进制发布；各服务启动时都使用这一份 SQL 与历史校验。
// 库存、订单、消费者和 shopctl 经 Apply 进入通用迁移器。

package migrations

import (
	"context"
	"database/sql"
	"embed"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/migrate"
)

// files 保留迁移的原始字节，供迁移器计算和核对校验和。
//
//go:embed *.sql
var files embed.FS

// Apply 在数据库命名锁内核对迁移历史并执行新 SQL，启动时限由调用方提供。
func Apply(ctx context.Context, database *sql.DB) error {
	return migrate.Apply(ctx, database, files, ".")
}
