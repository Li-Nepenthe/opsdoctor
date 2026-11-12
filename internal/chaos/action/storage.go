// 本文件：实验数据访问使用独立账号，并限定在登记的索引和商品缓存范围内。
// expectedIndex 在操作前核对索引结构，防止同名索引被误改。

package action

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/chaos/scenario"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/redis/go-redis/v9"
)

// index 不接受任意 SQL。两个索引的列顺序在代码里写死，恢复不会悄悄建错索引。
func (runner *Runner) index(ctx context.Context, step scenario.Step) error {
	database, err := mysqlx.Open(ctx, mysqlx.Config{DSN: runner.MySQLDSN, MaxOpenConns: 1, MaxIdleConns: 1, ConnMaxLifetime: time.Minute})
	if err != nil {
		return err
	}
	defer database.Close()
	exists, err := expectedIndex(ctx, database, step)
	if err != nil {
		return err
	}
	if step.Action == "restore_index" && exists {
		return nil
	}
	if step.Action == "drop_index" && !exists {
		return fmt.Errorf("目标索引原本不存在，拒绝把已有问题算成本次实验")
	}
	// ALTER TABLE 取得元数据锁。删除索引后，业务查询才会自然退化；这里不制造慢查询。
	statement := "ALTER TABLE stock DROP INDEX uk_stock_sku"
	if step.Table == "orders" {
		statement = "ALTER TABLE orders DROP INDEX idx_orders_user_created"
	}
	if step.Action == "restore_index" {
		statement = "ALTER TABLE stock ADD UNIQUE INDEX uk_stock_sku(sku_id)"
		if step.Table == "orders" {
			statement = "ALTER TABLE orders ADD INDEX idx_orders_user_created(user_id,created_at)"
		}
	}
	_, err = database.ExecContext(ctx, statement)
	if err != nil {
		return fmt.Errorf("执行索引变更：%w", err)
	}
	return nil
}

// expectedIndex 不只检查名字，还核对列顺序和唯一性，避免把同名但结构不同的索引误删。
func expectedIndex(ctx context.Context, database *sql.DB, step scenario.Step) (bool, error) {
	// 没有匹配索引时结果为 NULL，要与真实的空值或零值区分。
	var columns sql.NullString
	var nonUnique sql.NullInt64
	// 只读取元数据，不扫描业务记录，也不取得业务行锁。
	err := database.QueryRowContext(ctx, `SELECT GROUP_CONCAT(column_name ORDER BY seq_in_index), MIN(non_unique) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND index_name=?`, step.Table, step.Index).Scan(&columns, &nonUnique)
	if err != nil {
		return false, fmt.Errorf("读取索引状态：%w", err)
	}
	if !columns.Valid {
		return false, nil
	}
	wantColumns, wantUnique := "sku_id", int64(0)
	if step.Table == "orders" {
		wantColumns, wantUnique = "user_id,created_at", 1
	}
	if !nonUnique.Valid || columns.String != wantColumns || nonUnique.Int64 != wantUnique {
		return false, fmt.Errorf("目标索引的列顺序或唯一性不符合预期，停止变更")
	}
	return true, nil
}

// cache 使用 pipeline 一次发送多个独立命令，减少宿主机逐次往返造成的到期时间差。
func (runner *Runner) cache(ctx context.Context, step scenario.Step) error {
	// 使用业务 DB 0，但键名由固定前缀和受限商品编号组成，绝不调用 FLUSHDB。
	if runner.RedisPassword == "" {
		return fmt.Errorf("缓存实验需要 REDIS_PASSWORD")
	}
	client := redis.NewClient(&redis.Options{Addr: runner.RedisAddress, Password: runner.RedisPassword, DB: 0, DialTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second})
	defer client.Close()
	deadline := time.Now().Add(time.Duration(step.TTLSeconds) * time.Second)
	// 共用一个绝对到期时间。逐条使用相对 TTL 会把发送时间差带进实验。
	commands, err := client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for offset := 0; offset < step.Count; offset++ {
			key := "product:" + strconv.FormatInt(step.FirstSKUID+int64(offset), 10)
			if step.Action == "delete_cache" {
				pipe.Del(ctx, key)
			} else {
				pipe.ExpireAt(ctx, key, deadline)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("修改商品缓存：%w", err)
	}
	// 没改到任何已有缓存，就没有注入这个原因，不能靠后来的偶然波动通过验收。
	affected := int64(0)
	for _, command := range commands {
		switch result := command.(type) {
		case *redis.IntCmd:
			affected += result.Val()
		case *redis.BoolCmd:
			if result.Val() {
				affected++
			}
		}
	}
	if affected == 0 {
		return fmt.Errorf("没有命中已有商品缓存，请先用正常流量预热")
	}
	return nil
}
