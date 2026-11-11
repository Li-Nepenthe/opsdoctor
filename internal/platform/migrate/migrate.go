// 本文件：Apply 在服务启动时锁住当前库，核对历史后只执行新增的迁移。
// 这里保留自己的迁移器：评估过的 golang-migrate v4.20.1 固定等待锁 10 秒，无法按调用方要求取消等待，失败后还需人工处理 dirty 状态。
// 它也不校验已执行文件的内容；接管旧库时仍需维护这套历史检查。

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

// 锁等待只限制竞争时间，整个迁移的截止时间由调用方决定。
const (
	lockWaitSeconds = 30
	unlockTimeout   = 5 * time.Second // 调用方即使已取消，仍给解锁留一段有限的时间。
)

const databaseSQL = `SELECT DATABASE()`

const acquireLockSQL = `SELECT GET_LOCK(?, ?)`

const releaseLockSQL = `SELECT RELEASE_LOCK(?)`

const createHistorySQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version VARCHAR(255) NOT NULL PRIMARY KEY,
    checksum CHAR(64) NOT NULL,
    applied_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
)`

const historySQL = `
SELECT version, checksum
FROM schema_migrations
ORDER BY version`

const recordSQL = `
INSERT INTO schema_migrations (version, checksum)
VALUES (?, ?)`

// Apply 在同一 MySQL 会话中持锁执行新增迁移。调用方须禁用多语句，每份文件只含一条 SQL。
// DDL 成功后若记录历史失败，重试会再次执行这份文件，迁移 SQL 必须能重复执行。
func Apply(ctx context.Context, database *sql.DB, source fs.FS, directory string) (result error) {
	// 先检查所有文件，免得一个无效文件让其他启动实例白等锁。
	migrations, err := load(source, directory)
	if err != nil {
		return err
	}
	connection, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("租用迁移连接：%w", err)
	}
	// 归还连接失败不能盖住迁移本身的错误，两个都要带回去。
	defer func() { result = errors.Join(result, closeConnection(connection)) }()
	// 命名锁属于会话，加锁、执行迁移和解锁都必须用借出的这条连接。
	lockName, err := acquire(ctx, connection)
	if err != nil {
		return err
	}
	// 归还给连接池之前先解锁，避免下一个请求借到仍持锁的会话。
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

func acquire(ctx context.Context, connection *sql.Conn) (string, error) {
	var database string
	if err := connection.QueryRowContext(ctx, databaseSQL).Scan(&database); err != nil {
		return "", fmt.Errorf("读取迁移库名：%w", err)
	}
	// 同一个库使用同一个锁名，截取摘要使锁名满足 MySQL 的 64 字节限制。
	const lockHashBytes = 16
	digest := sha256.Sum256([]byte(database))
	name := fmt.Sprintf("opsdoctor:migrate:%x", digest[:lockHashBytes])
	var acquired sql.NullInt64
	err := connection.QueryRowContext(ctx, acquireLockSQL, name, lockWaitSeconds).Scan(&acquired)
	if err != nil || !acquired.Valid || acquired.Int64 != 1 {
		// 取消可能恰好发生在拿到锁之后。无法确认是否持锁时，要关闭这个会话。
		discardErr := discard(connection)
		if err != nil {
			return "", errors.Join(fmt.Errorf("获取迁移命名锁：%w", err), discardErr)
		}
		return "", errors.Join(fmt.Errorf("获取迁移命名锁超时或返回 NULL"), discardErr)
	}
	return name, nil
}

func readHistory(ctx context.Context, connection *sql.Conn) (history map[string]string, result error) {
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
		// 历史必须读全，读取中断时不能把剩下的旧版本当作新迁移。
		return nil, fmt.Errorf("遍历迁移历史：%w", err)
	}
	return history, nil
}

// verifyHistory 只允许在末尾追加迁移，让新建库和已有库按相同顺序升级。
func verifyHistory(migrations []migration, history map[string]string) error {
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
			// 历史记录缺少对应文件，和文件被改写一样，都必须停止迁移。
			return fmt.Errorf("已应用迁移 %s 被修改或移除；请追加新文件", name)
		}
	}
	return nil
}

func release(connection *sql.Conn, name string) error {
	// 迁移可能因调用方取消而结束，解锁需要独立的截止时间。
	ctx, cancel := context.WithTimeout(context.Background(), unlockTimeout)
	defer cancel()
	var released sql.NullInt64
	err := connection.QueryRowContext(ctx, releaseLockSQL, name).Scan(&released)
	if err == nil && released.Valid && released.Int64 == 1 {
		// 只有返回 1 才确认当前会话已释放命名锁。
		return nil
	}
	discardErr := discard(connection)
	if err != nil {
		return errors.Join(fmt.Errorf("释放迁移命名锁：%w", err), discardErr)
	}
	return errors.Join(fmt.Errorf("释放迁移命名锁返回异常结果"), discardErr)
}

// discard 让连接池关掉这条连接。解锁未确认时，会话可能仍持有命名锁。
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

func closeConnection(connection *sql.Conn) error {
	err := connection.Close()
	if errors.Is(err, sql.ErrConnDone) {
		// discard 可能已经关掉了这条连接。
		return nil
	}
	if err != nil {
		return fmt.Errorf("归还迁移连接：%w", err)
	}
	return nil
}
