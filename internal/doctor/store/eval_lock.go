// 本文件：保证同一批评测或同一份评分同时只有一个命令行进程负责。
//
// 本地进程退出以后，数据库会释放它的连接锁。第二个进程取得锁后才能检查旧
// 记录并继续。连接丢失时立即取消工作，不能继续拿过期的执行权创建付费请求。
//
// 主要内容：WithEvalLock 固定一条连接持锁、监测连接，并在结束时丢弃该连接。
// 阅读建议：锁连接不参与业务查询，其他查询仍从原连接池借连接。

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
	go func() {
		defer close(done)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				probe, stop := context.WithTimeout(ctx, time.Second)
				var owned sql.NullBool
				problem := connection.QueryRowContext(probe, "SELECT IS_USED_LOCK(?)=CONNECTION_ID()", name).Scan(&owned)
				stop()
				if problem != nil || !owned.Valid || !owned.Bool {
					cancel(fmt.Errorf("评测锁连接已丢失，已暂停，请检查数据库后续跑"))
					return
				}
			}
		}
	}()
	err = work(ctx)
	cause := context.Cause(ctx)
	cancel(nil)
	<-done // 监测退出后才能归还或丢弃连接，避免关闭与查询并发。
	if cause != nil {
		return cause
	}
	return err
}
