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

// databaseSQL 读取当前库名生成锁名；只读会话状态，没有表索引或行锁。
const databaseSQL = `SELECT DATABASE()`

// acquireLockSQL 获取连接级命名锁；无表索引、无行锁，同库的迁移在这里排队。
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
// 同库可并发调用，由数据库命名锁串行化；函数会等待锁和 SQL，ctx 控制运行预算。
// 文件、历史不一致、SQL、取消或清理失败均返回错误；DDL 成功但记账失败时需重跑可重入 SQL。
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
	defer func() { result = errors.Join(result, closeConnection(connection)) }()
	// 第 2 步：锁属于会话，不能使用连接池随机分配的 database.Exec 执行迁移。
	lockName, err := acquire(ctx, connection)
	if err != nil {
		return err
	}
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

// acquire 根据当前库名隔离命名锁；返回锁名或明确的获取错误，不记录连接串。
func acquire(ctx context.Context, connection *sql.Conn) (string, error) {
	var database string
	if err := connection.QueryRowContext(ctx, databaseSQL).Scan(&database); err != nil {
		return "", fmt.Errorf("读取迁移库名：%w", err)
	}
	// 取哈希前 16 字节，锁名保持在 MySQL 的 64 字节限制内；同库始终得到同一名称。
	const lockHashBytes = 16
	digest := sha256.Sum256([]byte(database))
	name := fmt.Sprintf("opsdoctor:migrate:%x", digest[:lockHashBytes])
	var acquired sql.NullInt64
	err := connection.QueryRowContext(ctx, acquireLockSQL, name, lockWaitSeconds).Scan(&acquired)
	if err != nil || !acquired.Valid || acquired.Int64 != 1 {
		// 取消恰好发生在拿到锁之后时，不能确定会话是否持锁；直接丢弃物理连接最可靠。
		discardErr := discard(connection)
		if err != nil {
			return "", errors.Join(fmt.Errorf("获取迁移命名锁：%w", err), discardErr)
		}
		return "", errors.Join(fmt.Errorf("获取迁移命名锁超时或返回 NULL"), discardErr)
	}
	return name, nil
}

// readHistory 读取已执行版本与校验和；同步查询，所有读取和关闭错误都返回给调用方。
func readHistory(ctx context.Context, connection *sql.Conn) (history map[string]string, result error) {
	// rows 可能在遍历中途遇到网络错误，因此 Scan、Err 和 Close 三处都要处理。
	rows, err := connection.QueryContext(ctx, historySQL)
	if err != nil {
		return nil, fmt.Errorf("读取迁移历史：%w", err)
	}
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
		return nil, fmt.Errorf("遍历迁移历史：%w", err)
	}
	return history, nil
}

// verifyHistory 拒绝改写、移除或在旧版本前插入迁移；不联网，不修改输入数据。
func verifyHistory(migrations []migration, history map[string]string) error {
	// 先找数据库已见过的最高版本，再核对本地文件，拒绝悄悄在旧版本前补执行 SQL。
	files := make(map[string]string)
	latest := ""
	for name := range history {
		if name > latest {
			latest = name
		}
	}
	for _, item := range migrations {
		files[item.name] = item.checksum
		if _, exists := history[item.name]; !exists && item.name < latest {
			return fmt.Errorf("迁移 %s 不能插入已应用版本之前", item.name)
		}
	}
	for name, checksum := range history {
		if files[name] != checksum {
			return fmt.Errorf("已应用迁移 %s 被修改或移除；请追加新文件", name)
		}
	}
	return nil
}

// release 使用独立超时释放命名锁；失败时丢弃物理连接，避免把仍持锁的会话放回连接池。
func release(connection *sql.Conn, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), unlockTimeout)
	defer cancel()
	var released sql.NullInt64
	err := connection.QueryRowContext(ctx, releaseLockSQL, name).Scan(&released)
	if err == nil && released.Valid && released.Int64 == 1 {
		return nil
	}
	discardErr := discard(connection)
	if err != nil {
		return errors.Join(fmt.Errorf("释放迁移命名锁：%w", err), discardErr)
	}
	return errors.Join(fmt.Errorf("释放迁移命名锁返回异常结果"), discardErr)
}

// discard 用 database/sql 的标准协议废弃连接；ErrBadConn 是丢弃指令，不是需要继续上报的新故障。
func discard(connection *sql.Conn) error {
	err := connection.Raw(func(any) error { return driver.ErrBadConn })
	if errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) {
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
		return nil
	}
	if err != nil {
		return fmt.Errorf("归还迁移连接：%w", err)
	}
	return nil
}
