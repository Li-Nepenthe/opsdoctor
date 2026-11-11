// 本文件：库存扣减和归还通过单条条件更新完成。更新没有命中时，再查询商品是否存在以区分失败原因。
// 从 Deduct 进入 change，可以区分库存竞争、缺失记录和驱动故障。

package inventory

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/go-sql-driver/mysql"
)

// getStockSQL 通过唯一索引 uk_stock_sku 定位库存。
// 查询得到的是当时的快照，并未预留这些库存。
const getStockSQL = `
SELECT sku_id, available FROM stock WHERE sku_id = ?`

// deductStockSQL 把库存判断和减法放在同一条 UPDATE 中，避免两个请求都读到最后一件。
// uk_stock_sku 定位目标行，InnoDB 对更新行加排他锁。等待者拿到锁后会重新判断 available。
// 默认 REPEATABLE READ 下，缺索引可能扫描并锁住大量记录，因此必须保留独立的 SKU 索引。
const deductStockSQL = `
UPDATE stock SET available = available - ?, updated_at = ?
WHERE sku_id = ? AND available >= ?`

// releaseStockSQL 按 uk_stock_sku 更新。上界按 int32 的最大值计算，避免归还时溢出。
const releaseStockSQL = `
UPDATE stock SET available = available + ?, updated_at = ?
WHERE sku_id = ? AND available <= 2147483647 - ?`

// MySQLStockRepository 只持有可并发使用的连接池。关闭池的责任属于创建它的 app。
type MySQLStockRepository struct {
	database *sql.DB
}

// NewMySQLStockRepository 复用库存服务的连接池。
func NewMySQLStockRepository(database *sql.DB) *MySQLStockRepository {
	return &MySQLStockRepository{database: database}
}

// GetStock 使用请求 ctx 查询。无行时返回领域错误，网络错误保留原因并标记暂时不可用。
func (repository *MySQLStockRepository) GetStock(ctx context.Context, skuID int64) (Stock, error) {
	var stock Stock
	err := repository.database.QueryRowContext(ctx, getStockSQL, skuID).Scan(&stock.SKUID, &stock.Available)
	if errors.Is(err, sql.ErrNoRows) {
		// 只有缺少记录才表示商品不存在，连接和查询错误须另行分类。
		return Stock{}, fmt.Errorf("查询 sku=%d：%w", skuID, ErrNotFound)
	}
	if err != nil {
		return Stock{}, fmt.Errorf("查询 sku=%d：%w", skuID, classifyDatabaseError(ctx, err))
	}
	return stock, nil
}

// Deduct 等待单条条件更新完成。影响行数为 0 时再区分库存不足与不存在，不根据预读来扣减。
func (repository *MySQLStockRepository) Deduct(ctx context.Context, skuID int64, quantity int32) error {
	return repository.change(ctx, deductStockSQL, skuID, quantity, ErrInsufficientStock)
}

// Release 以同样的原子更新方式归还。超过 int32 上限属于参数无法满足当前库存范围。
func (repository *MySQLStockRepository) Release(ctx context.Context, skuID int64, quantity int32) error {
	return repository.change(ctx, releaseStockSQL, skuID, quantity, ErrInvalidArgument)
}

// change 只接受本文件的两条更新语句，商品和数量由占位符传入。
func (repository *MySQLStockRepository) change(ctx context.Context, statement string, skuID int64, quantity int32, rejected Error) error {
	result, err := repository.database.ExecContext(ctx, statement, quantity, time.Now().UTC(), skuID, quantity)
	if err != nil {
		return fmt.Errorf("变更 sku=%d：%w", skuID, classifyDatabaseError(ctx, err))
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取 sku=%d 的影响行数：%w", skuID, err)
	}
	if affected > 0 {
		return nil
	}
	// 更新失败后才查存在性。这次查询只帮助选择错误码，绝不再依据查询结果执行扣减。
	if _, err := repository.GetStock(ctx, skuID); err != nil {
		return err
	}
	return fmt.Errorf("变更 sku=%d 的条件不满足：%w", skuID, rejected)
}

// classifyDatabaseError 隔离驱动类型。业务层只识别领域错误和标准 context 错误。
func classifyDatabaseError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		// 驱动可能在取消连接时返回网络错误，优先保留请求结束的真正原因。
		return ctx.Err()
	}
	var networkError net.Error
	var mysqlError *mysql.MySQLError
	unavailable := errors.As(err, &networkError) || errors.Is(err, driver.ErrBadConn) ||
		errors.Is(err, mysql.ErrInvalidConn) || errors.Is(err, io.EOF)
	if errors.As(err, &mysqlError) {
		// 只把连接容量、断连、死锁和锁等待归为暂时失败。语法/权限错误应留为 INTERNAL。
		switch mysqlError.Number {
		case 1040, 1205, 1213, 2002, 2003, 2006, 2013:
			unavailable = true
		}
	}
	if unavailable {
		// 保留原始故障，调用方同时能识别暂时不可用。
		return errors.Join(ErrUnavailable, err)
	}
	return err
}
