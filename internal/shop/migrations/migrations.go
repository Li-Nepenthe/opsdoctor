// 本文件：把 shop 的 SQL 文件嵌入并交给通用迁移器。
//
// 库存服务与 shopctl 都调用 Apply，所以无论先启动哪个入口，表结构都一致。
// 先看 go:embed 对文件路径的约束，再看 Apply 如何复用 platform/migrate。这里不重复实现锁和历史表。
//
// 主要内容：
//   - Apply：使用阶段 0 的命名锁和校验和机制应用迁移。调用方负责提供启动超时并关闭连接池。

package migrations

import (
	"context"
	"database/sql"
	"embed"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/migrate"
)

// files 由编译器填充，运行期间只读。go:embed 是编译指令，不是启动时去磁盘读文件。
// 这里的包级值仅作为不可变资源使用，各服务共享相同的迁移内容。
//
//go:embed *.sql
var files embed.FS

// Apply 使用阶段 0 的命名锁和校验和机制应用迁移。调用方负责提供启动超时并关闭连接池。
func Apply(ctx context.Context, database *sql.DB) error {
	return migrate.Apply(ctx, database, files, ".")
}
