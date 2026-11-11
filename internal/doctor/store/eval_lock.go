// 本文件：同一批评测或同一份评分只能有一个执行者。MySQL 命名锁由独占的物理连接持有，其余业务查询走连接池。
// 进程退出会释放锁。持锁连接中断时必须取消评测，防止旧进程继续创建付费请求。

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"time"
)

// WithEvalLock 不等待另一个操作者，重复启动时直接说明已有执行者。
func (store *Store) WithEvalLock(parent context.Context, key string, work func(context.Context) error) error {
	connection, err := store.database.Conn(parent)
	if err != nil {
		return err
	}
	// 命名锁属于物理连接。丢弃连接会释放锁，不能把仍持锁的会话放回池里。
	defer connection.Close()
	defer func() { _ = connection.Raw(func(any) error { return driver.ErrBadConn }) }()
	digest := sha256.Sum256([]byte(key))
	name := hex.EncodeToString(digest[:])
	var acquired sql.NullInt64
	if err = connection.QueryRowContext(parent, "SELECT GET_LOCK(?,0)", name).Scan(&acquired); err != nil {
		return err
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return fmt.Errorf("这批评测已有执行者，请勿同时启动两个命令")
	}
	ctx, cancel := context.WithCancelCause(parent)
	done := make(chan struct{})
	go monitorEvalLock(ctx, cancel, connection, name, done)
	err = work(ctx)
	cause := context.Cause(ctx)
	cancel(nil)
	<-done // 监测退出后才能归还或丢弃连接，避免关闭与查询并发。
	if cause != nil {
		return cause
	}
	return err
}

// monitorEvalLock 在持锁连接上复核归属。退出后通知调用方，此时才能丢弃连接。
func monitorEvalLock(ctx context.Context, cancel context.CancelCauseFunc, connection *sql.Conn, name string, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		probe, stop := context.WithTimeout(ctx, time.Second)
		var owned sql.NullBool
		err := connection.QueryRowContext(probe, "SELECT IS_USED_LOCK(?)=CONNECTION_ID()", name).Scan(&owned)
		stop()
		if err != nil || !owned.Valid || !owned.Bool {
			cancel(fmt.Errorf("评测锁连接已丢失，已暂停，请检查数据库后续跑"))
			return
		}
	}
}
