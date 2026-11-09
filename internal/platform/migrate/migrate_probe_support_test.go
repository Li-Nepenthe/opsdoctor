// 本文件：给迁移兼容性探针准备隔离数据库、内存 SQL 文件和可公开的观测结果。
//
// 这些辅助函数只编入测试，不接入 Apply。测试库白名单沿用已有集成测试，
// 生产 embed 变量不导出，所以从同一批仓库 SQL 读取原字节，仅在 MapFS 中改变文件名。
// 先看 probeDatabase 的清理边界，再看 probeOpen 和 probeLog；接管 SQL 只是待验证的候选做法。

package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	gm "github.com/golang-migrate/migrate/v4"
	gmdb "github.com/golang-migrate/migrate/v4/database"
	gmmysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// probeLibrary 持有一条真实连接和库实例；close 只在 Up 的工作者退出后调用。
type probeLibrary struct {
	migration  *gm.Migrate // 待观测的第三方实例，不包装重试、接管或取消行为。
	connection *sql.Conn   // 并发接管时，每个候选启动者都使用自己的连接。
	closed     bool        // 测试内显式关闭后，Cleanup 不再重复关闭。
}

// probeDatabase 只连接已有白名单中的迁移测试库；包内探针不使用 Parallel。
func probeDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db := integrationDatabase(t)
	// 归还时关闭物理连接，防止探针故意触发的失败把会话锁带入下一个用例。
	db.SetMaxIdleConns(0)
	probeReset(t, db)
	t.Cleanup(func() { probeReset(t, db) })
	return db
}

// probeReset 清空专用测试库，允许 doctor 与 shop 的同名表分别从空库开始。
func probeReset(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal("探针清理无法租用连接：", probeError(err))
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error(probeError(err))
		}
	}()
	var name string
	if err := conn.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&name); err != nil || name != "opsdoctor_migrate_test" {
		t.Fatal("探针只允许清理 opsdoctor_migrate_test")
	}
	// 表名来自刚确认过的库；标识符仍转义。关闭外键检查只作用于这一条清理连接。
	rows, err := conn.QueryContext(ctx, "SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE'")
	if err != nil {
		t.Fatal(probeError(err))
	}
	var names []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			_ = rows.Close()
			t.Fatal(probeError(err))
		}
		names = append(names, table)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		t.Fatal(probeError(err))
	}
	if _, err := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		t.Fatal(probeError(err))
	}
	defer func() {
		if _, err := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1"); err != nil {
			t.Error(probeError(err))
		}
	}()
	for _, table := range names {
		if _, err := conn.ExecContext(ctx, "DROP TABLE `"+strings.ReplaceAll(table, "`", "``")+"`"); err != nil {
			t.Fatal(probeError(err))
		}
	}
}

// probeRepositoryFiles 读取生产 embed 对应的 SQL 原件，不复制一份可能过期的测试 SQL。
func probeRepositoryFiles(t *testing.T, directory string, expected int) fstest.MapFS {
	t.Helper()
	source := os.DirFS(directory)
	names, err := fs.Glob(source, "*.sql")
	if err != nil || len(names) != expected {
		t.Fatalf("SQL 文件数=%d，预期=%d", len(names), expected)
	}
	files := fstest.MapFS{}
	for index, name := range names {
		// 当前两套 SQL 均连续编号；不要把目录中的 README 算成一条迁移。
		if !strings.HasPrefix(name, fmt.Sprintf("%04d_", index+1)) {
			t.Fatal("源文件编号不连续：", name)
		}
		data, err := fs.ReadFile(source, name)
		if err != nil {
			t.Fatal("读取探针 SQL 失败")
		}
		files[name] = &fstest.MapFile{Data: data}
	}
	return files
}

// probeFiles 为小型反例生成旧实现接受的名字，每个文件仍只有一条 SQL。
func probeFiles(statements ...string) fstest.MapFS {
	files := fstest.MapFS{}
	for index, statement := range statements {
		files[fmt.Sprintf("%04d_probe.sql", index+1)] = &fstest.MapFile{Data: []byte(statement)}
	}
	return files
}

// probeUpFiles 只在内存中插入 .up，复制原字节，磁盘文件和旧 Apply 的输入不变。
func probeUpFiles(files fstest.MapFS) fstest.MapFS {
	up := fstest.MapFS{}
	for name, file := range files {
		up[strings.TrimSuffix(name, ".sql")+".up.sql"] = &fstest.MapFile{Data: append([]byte(nil), file.Data...)}
	}
	return up
}

// probeOpen 按方案使用 WithConnection；不启用多语句，不加生产适配层。
func probeOpen(t *testing.T, ctx context.Context, db *sql.DB, files fstest.MapFS, table string) (*probeLibrary, error) {
	t.Helper()
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	driver, err := gmmysql.WithConnection(ctx, conn, &gmmysql.Config{MigrationsTable: table})
	if err != nil {
		return nil, errors.Join(err, conn.Close())
	}
	source, err := iofs.New(probeUpFiles(files), ".")
	if err != nil {
		return nil, errors.Join(err, driver.Close())
	}
	migration, err := gm.NewWithInstance("iofs", source, "mysql", driver)
	if err != nil {
		return nil, errors.Join(err, source.Close(), driver.Close())
	}
	instance := &probeLibrary{migration: migration, connection: conn}
	// 测试清理发生在所有同步调用或已 join 的工作者之后，避免关闭正在使用的连接。
	t.Cleanup(func() { instance.close(t) })
	return instance, nil
}

// close 关闭库的资源；零空闲连接池同时保证底层 MySQL 会话被释放。
func (instance *probeLibrary) close(t *testing.T) {
	t.Helper()
	if instance.closed {
		return
	}
	sourceErr, databaseErr := instance.migration.Close()
	instance.closed = true
	if err := errors.Join(sourceErr, databaseErr); err != nil {
		t.Error("关闭探针实例：", probeError(err))
	}
}

// probeMustOpen 只用于预期能初始化的准备步骤；构造器锁探针直接观察 probeOpen 的错误。
func probeMustOpen(t *testing.T, ctx context.Context, db *sql.DB, files fstest.MapFS, table string) *probeLibrary {
	t.Helper()
	instance, err := probeOpen(t, ctx, db, files, table)
	if err != nil {
		t.Fatal("初始化探针失败：", probeError(err))
	}
	return instance
}

// probeAdopt 仅复现候选接管 SQL；version 来自测试刚执行并核对的旧历史，不是生产接管实现。
func probeAdopt(ctx context.Context, conn *sql.Conn, version int) (int64, error) {
	// NOT EXISTS 只在新表为空时插入，故意保留数据库原始竞争结果供 CI 观测。
	result, err := conn.ExecContext(ctx, "INSERT INTO schema_version (version, dirty) SELECT ?, 0 FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM schema_version)", version)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// probeCount 读取测试中明确指定的小型计数，不把数据库响应正文写入日志。
func probeCount(t *testing.T, db *sql.DB, query string) int64 {
	t.Helper()
	var count int64
	if err := db.QueryRowContext(t.Context(), query).Scan(&count); err != nil {
		t.Fatal("探针计数失败：", probeError(err))
	}
	return count
}

// probeSchema 对业务表的 SHOW CREATE TABLE 做哈希，比较新旧迁移得到的真实结构。
func probeSchema(t *testing.T, db *sql.DB) map[string]any {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME NOT IN ('schema_migrations', 'schema_version') ORDER BY TABLE_NAME")
	if err != nil {
		t.Fatal(probeError(err))
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			t.Fatal(probeError(err))
		}
		names = append(names, name)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		t.Fatal(probeError(err))
	}
	// 不输出表结构正文，固定排序后比对哈希；迁移历史表因两种实现的格式不同而排除。
	var definitions strings.Builder
	for _, name := range names {
		var table, definition string
		if err := db.QueryRowContext(t.Context(), "SHOW CREATE TABLE `"+strings.ReplaceAll(name, "`", "``")+"`").Scan(&table, &definition); err != nil {
			t.Fatal(probeError(err))
		}
		fmt.Fprintf(&definitions, "%s\n%s\n", table, definition)
	}
	return map[string]any{"tables": len(names), "sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(definitions.String())))}
}

// probeState 记录库真实读出的版本和 dirty，不把没有版本误写成版本 0。
func probeState(instance *probeLibrary) map[string]any {
	version, dirty, err := instance.migration.Version()
	return map[string]any{"version": version, "dirty": dirty, "version_error": probeError(err)}
}

// probeError 记录可区分的错误类型和 MySQL 编号，省去可能含地址或 SQL 正文的库错误文本。
func probeError(err error) string {
	if err == nil {
		return "nil"
	}
	for _, known := range []struct {
		value error
		name  string
	}{
		{context.Canceled, "context.Canceled"}, {context.DeadlineExceeded, "context.DeadlineExceeded"},
		{gm.ErrNoChange, "migrate.ErrNoChange"}, {gm.ErrNilVersion, "migrate.ErrNilVersion"},
		{gmdb.ErrLocked, "database.ErrLocked"}, {gm.ErrLocked, "migrate.ErrLocked"}, {gm.ErrLockTimeout, "migrate.ErrLockTimeout"},
	} {
		if errors.Is(err, known.value) {
			return known.name
		}
	}
	var dirty gm.ErrDirty
	if errors.As(err, &dirty) {
		return fmt.Sprintf("migrate.ErrDirty(%d)", dirty.Version)
	}
	var mysqlErr *mysqldriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return fmt.Sprintf("mysql.%d", mysqlErr.Number)
	}
	// v4.20.1 的 database.Error 不实现 Unwrap，必须显式读 OrigErr 才能看到 SQL 错误编号。
	switch typed := err.(type) {
	case gmdb.Error:
		return "database.Error/" + probeError(typed.OrigErr)
	case *gmdb.Error:
		return "database.Error/" + probeError(typed.OrigErr)
	}
	if strings.Contains(err.Error(), "被修改或移除") {
		return "history_checksum_mismatch"
	}
	return fmt.Sprintf("%T", err)
}

// probeLog 输出独立 JSON 行，CI 使用 -json 时仍能从 Output 字段无损提取差异表。
func probeLog(t *testing.T, name string, observation map[string]any) {
	t.Helper()
	observation["case"] = name
	encoded, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("MIGRATE_PROBE " + string(encoded))
}
