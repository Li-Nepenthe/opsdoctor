// 本文件：在同一 MySQL 会话中串行执行迁移并记录历史。
//
// 命名锁属于连接，所以 Apply 必须先借出专用连接，再获取锁、核对历史和执行 SQL。
// 先通读 Apply 的资源回收，再看 acquire、readHistory、verifyHistory；最后读 release/discard，理解释放锁失败时为何不能归还脏连接。

package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io/fs"
	"time"
)

// 锁等待只限制竞争时间；整个迁移的运行预算由调用方传入的 ctx 决定。
const (
	lockWaitSeconds = 30              // 为另一个正在启动的实例留出迁移时间，超过就明确失败。
	unlockTimeout   = 5 * time.Second // 清理独立于已取消的请求，但不能无限等待断开的数据库。
)

// databaseSQL 查询这条连接当前使用的数据库，后面据此生成锁名。
// DATABASE() 直接返回会话信息，不查询任何表，因此不涉及表索引，也不会给数据行加锁。
const databaseSQL = `SELECT DATABASE()`

// acquireLockSQL 申请命名锁，让迁移同一个数据库的进程依次执行。
// 这是 MySQL 按名字管理的会话锁，不查询表、不使用索引，也不是锁住某条数据的行锁。
const acquireLockSQL = `SELECT GET_LOCK(?, ?)`

// releaseLockSQL 释放当前连接持有的锁；必须与 GET_LOCK 使用同一个物理会话。
const releaseLockSQL = `SELECT RELEASE_LOCK(?)`

// createHistorySQL 创建技术元数据表；DDL 有元数据锁且会隐式提交，IF NOT EXISTS 支持重试。
const createHistorySQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version VARCHAR(255) NOT NULL PRIMARY KEY,
    checksum CHAR(64) NOT NULL,
    applied_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
)`

// historySQL 读取已应用记录；按主键顺序扫描小型历史表，不使用 FOR UPDATE，不加行锁。
const historySQL = `
SELECT version, checksum
FROM schema_migrations
ORDER BY version`

// recordSQL 按主键记录成功迁移；插入时 InnoDB 对新记录加锁，唯一主键防止重复记账。
const recordSQL = `
INSERT INTO schema_migrations (version, checksum)
VALUES (?, ?)`

// Apply 在同一数据库连接上持锁、检查历史、执行新迁移并记录结果。
// database 必须由 mysqlx.Open 创建以关闭多语句；source 通常是 embed.FS，directory 是其中的路径。
// 同库可并发调用，命名锁保证每次只有一个调用执行迁移；等待锁和执行 SQL 都受 ctx 限制。
// 文件、历史不一致、SQL、取消或清理失败均返回错误；DDL 成功但记账失败时，重跑的 SQL 必须能重复执行。
// (result error) 给返回值起了名字：return 先给 result 赋值，再执行 defer，最后才把结果交给调用方。
// 所以 defer 里的闭包还能修改 result，让调用方同时知道迁移和清理是否失败。
func Apply(ctx context.Context, database *sql.DB, source fs.FS, directory string) (result error) {
	// 第 1 步：先加载所有文件，避免拿到锁之后才发现文件名不合法。
	migrations, err := load(source, directory)
	if err != nil {
		return err
	}
	connection, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("租用迁移连接：%w", err)
	}
	// errors.Join 把多个错误合成一个，忽略 nil，全部为 nil 时返回 nil；errors.Is/As 仍能找到其中的原因。
	// 这里保留迁移本身的错误，同时带上归还连接的错误，避免清理失败覆盖最初的故障。
	defer func() { result = errors.Join(result, closeConnection(connection)) }()
	// 第 2 步：锁属于会话，不能使用连接池随机分配的 database.Exec 执行迁移。
	lockName, err := acquire(ctx, connection)
	if err != nil {
		return err
	}
	// defer 按登记的相反顺序执行：先释放锁，再归还连接，避免别人借到一条还持锁的连接。
	defer func() { result = errors.Join(result, release(connection, lockName)) }()
	if _, err := connection.ExecContext(ctx, createHistorySQL); err != nil {
		return fmt.Errorf("创建迁移历史表：%w", err)
	}
	history, err := readHistory(ctx, connection)
	if err != nil {
		return err
	}
	if err := verifyHistory(migrations, history); err != nil {
		return err
	}
	// 第 3 步：已应用的不再执行，新文件执行成功后才写历史；不伪称 DDL 能随事务回滚。
	for _, item := range migrations {
		if _, exists := history[item.name]; exists {
			// 前面已经核对过文件校验和；历史里有这一项，说明它已成功执行，不应再改一次数据库。
			continue
		}
		if _, err := connection.ExecContext(ctx, item.statement); err != nil {
			return fmt.Errorf("执行迁移 %s：%w", item.name, err)
		}
		if _, err := connection.ExecContext(ctx, recordSQL, item.name, item.checksum); err != nil {
			return fmt.Errorf("记录迁移 %s：%w", item.name, err)
		}
	}
	return nil
}

// acquire 把当前库名放进锁名，迁移 shop 时不必阻挡另一个独立数据库的迁移。
func acquire(ctx context.Context, connection *sql.Conn) (string, error) {
	var database string
	if err := connection.QueryRowContext(ctx, databaseSQL).Scan(&database); err != nil {
		return "", fmt.Errorf("读取迁移库名：%w", err)
	}
	// 取哈希前 16 字节，锁名保持在 MySQL 的 64 字节限制内；同库始终得到同一名称。
	const lockHashBytes = 16
	digest := sha256.Sum256([]byte(database))
	name := fmt.Sprintf("opsdoctor:migrate:%x", digest[:lockHashBytes])
	// sql.NullInt64 用 Int64 保存整数，用 Valid 区分“有值”和 SQL NULL，类似 Java 可空 Long 的用途。
	// GET_LOCK 成功返回 1、超时返回 0、出错可能返回 NULL；普通 int64 无法表达这三种情况。
	var acquired sql.NullInt64
	err := connection.QueryRowContext(ctx, acquireLockSQL, name, lockWaitSeconds).Scan(&acquired)
	if err != nil || !acquired.Valid || acquired.Int64 != 1 {
		// 取消恰好发生在拿到锁之后时，不能确定会话是否持锁；直接丢弃物理连接最可靠。
		discardErr := discard(connection)
		if err != nil {
			// 查询失败时保留驱动给出的原因；只有查到了结果，才用下面的超时或 NULL 提示。
			return "", errors.Join(fmt.Errorf("获取迁移命名锁：%w", err), discardErr)
		}
		return "", errors.Join(fmt.Errorf("获取迁移命名锁超时或返回 NULL"), discardErr)
	}
	return name, nil
}

// readHistory 把已执行版本与校验和取到内存，后续才能判断迁移是否被偷偷改写或删除。
func readHistory(ctx context.Context, connection *sql.Conn) (history map[string]string, result error) {
	// rows 可能在遍历中途遇到网络错误，因此 Scan、Err 和 Close 三处都要处理。
	rows, err := connection.QueryContext(ctx, historySQL)
	if err != nil {
		return nil, fmt.Errorf("读取迁移历史：%w", err)
	}
	// 和 Apply 的具名返回值一样，先确定查询结果，再让 defer 把关闭游标时的错误补进 result。
	defer func() { result = errors.Join(result, rows.Close()) }()
	history = make(map[string]string)
	for rows.Next() {
		var name, checksum string
		if err := rows.Scan(&name, &checksum); err != nil {
			return nil, fmt.Errorf("解析迁移历史：%w", err)
		}
		history[name] = checksum
	}
	if err := rows.Err(); err != nil {
		// Next 返回 false 既可能表示读完，也可能表示读到一半断网；Err 用来区分这两种情况。
		return nil, fmt.Errorf("遍历迁移历史：%w", err)
	}
	return history, nil
}

// verifyHistory 要求历史只能向后追加；否则一台老实例和一台新实例可能认为自己执行过不同的表结构。
func verifyHistory(migrations []migration, history map[string]string) error {
	// 先找数据库已见过的最高版本，再核对本地文件，拒绝悄悄在旧版本前补执行 SQL。
	files := make(map[string]string)
	latest := ""
	for name := range history {
		if name > latest {
			// 文件使用等宽的四位序号，按名称比较就能找到最后一个已执行的迁移。
			latest = name
		}
	}
	for _, item := range migrations {
		files[item.name] = item.checksum
		if _, exists := history[item.name]; !exists && item.name < latest {
			// 新迁移只能排在末尾；在旧版本之间补文件，会让新旧数据库经历不同的执行顺序。
			return fmt.Errorf("迁移 %s 不能插入已应用版本之前", item.name)
		}
	}
	for name, checksum := range history {
		if files[name] != checksum {
			// 文件不存在时 map 返回空字符串，也会与已保存的校验和不同，因此同时发现删除和改写。
			return fmt.Errorf("已应用迁移 %s 被修改或移除；请追加新文件", name)
		}
	}
	return nil
}

// release 使用独立超时释放命名锁；失败时丢弃物理连接，避免把仍持锁的会话放回连接池。
func release(connection *sql.Conn, name string) error {
	// 原迁移可能因为 ctx 取消而退出；另建短时限，才能真正向数据库发送 RELEASE_LOCK。
	ctx, cancel := context.WithTimeout(context.Background(), unlockTimeout)
	defer cancel()
	var released sql.NullInt64
	err := connection.QueryRowContext(ctx, releaseLockSQL, name).Scan(&released)
	if err == nil && released.Valid && released.Int64 == 1 {
		// 只有返回 1 才确认锁已由这条连接释放；0 或 NULL 都不能作为归还连接的依据。
		return nil
	}
	// 无法确认解锁成功，就让数据库关闭整个会话；会话结束时，它持有的命名锁也随之释放。
	discardErr := discard(connection)
	if err != nil {
		return errors.Join(fmt.Errorf("释放迁移命名锁：%w", err), discardErr)
	}
	return errors.Join(fmt.Errorf("释放迁移命名锁返回异常结果"), discardErr)
}

// discard 通知连接池丢弃这条连接，避免别的请求复用一条可能仍持锁的会话。
func discard(connection *sql.Conn) error {
	// Raw 临时把底层驱动连接交给回调；回调返回 driver.ErrBadConn 时，database/sql 会关闭它，不放回池中。
	// 我们不需要操作驱动对象，只借这个约定废弃连接；普通 connection.Close() 通常只是把连接归还给池。
	err := connection.Raw(func(any) error { return driver.ErrBadConn })
	if errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) {
		// ErrBadConn 是我们主动发出的丢弃指令，ErrConnDone 表示已经关闭；两者都达到了清理目的。
		return nil
	}
	if err != nil {
		return fmt.Errorf("丢弃迁移连接：%w", err)
	}
	return nil
}

// closeConnection 归还租用连接；已经被 discard 关闭是预期状态，其他清理错误保留。
func closeConnection(connection *sql.Conn) error {
	err := connection.Close()
	if errors.Is(err, sql.ErrConnDone) {
		// 解锁失败时 discard 可能已经关掉连接，defer 再归还它不算第二次故障。
		return nil
	}
	if err != nil {
		return fmt.Errorf("归还迁移连接：%w", err)
	}
	return nil
}
