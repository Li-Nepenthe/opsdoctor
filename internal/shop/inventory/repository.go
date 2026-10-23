// 本文件：库存表的查询与原子更新。
//
// 请求沿 grpc_server.go 到 service.go，再到本文件执行 SQL。集中放置 SQL 方便逐句学习与排查。
// 先读 deductStockSQL 为什么依赖 uk_stock_sku，再看 change 如何用影响行数分类失败，最后看驱动错误的转换。
//
// 主要内容：
//   - NewMySQLStockRepository：借用 app 已建好的连接池，避免每个仓储或请求都另开一组数据库连接。
//   - GetStock：使用请求 ctx 查询。无行时返回领域错误，网络错误保留原因并标记暂时不可用。
//   - Deduct：等待单条条件更新完成。影响行数为 0 时再区分库存不足与不存在，不根据预读来扣减。
//   - Release：以同样的原子更新方式归还。超过 int32 上限属于参数无法满足当前库存范围。
//   - change：共用执行和错误分类。statement 只来自本文件常量，用户输入始终通过占位符传递。
//   - classifyDatabaseError：隔离驱动类型。业务层只识别领域错误和标准 context 错误。

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

// getStockSQL 通过唯一索引 uk_stock_sku 定位一行，再回表读取 available。
// 普通 SELECT 使用一致性读，不主动持有排他行锁。返回后库存仍可能被其他请求修改。
const getStockSQL = `
SELECT sku_id, available FROM stock WHERE sku_id = ?`

// deductStockSQL 把库存判断和减法放在同一条 UPDATE 中，避免两个请求都读到最后一件。
// uk_stock_sku 定位目标行，InnoDB 对更新行加排他锁。等待者拿到锁后会重新判断 available。
// 默认 REPEATABLE READ 下，缺索引可能扫描并锁住大量记录，因此必须保留独立的 SKU 索引。
const deductStockSQL = `
UPDATE stock SET available = available - ?, updated_at = ?
WHERE sku_id = ? AND available >= ?`

// releaseStockSQL 也按 uk_stock_sku 加锁更新。上界条件防止整数溢出变成数据库内部错误。
// 上界由 int32 契约决定，2147483647 是其最大值，不是可调的业务库存阈值。
const releaseStockSQL = `
UPDATE stock SET available = available + ?, updated_at = ?
WHERE sku_id = ? AND available <= 2147483647 - ?`

// MySQLStockRepository 只持有可并发使用的连接池。关闭池的责任属于创建它的 app。
type MySQLStockRepository struct {
	database *sql.DB // database 是连接池，不是一条连接；每次查询由 database/sql 借还连接。
}

// NewMySQLStockRepository 借用 app 已建好的连接池，避免每个仓储或请求都另开一组数据库连接。
func NewMySQLStockRepository(database *sql.DB) *MySQLStockRepository {
	return &MySQLStockRepository{database: database}
}

// GetStock 使用请求 ctx 查询。无行时返回领域错误，网络错误保留原因并标记暂时不可用。
func (repository *MySQLStockRepository) GetStock(ctx context.Context, skuID int64) (Stock, error) {
	var stock Stock
	err := repository.database.QueryRowContext(ctx, getStockSQL, skuID).Scan(&stock.SKUID, &stock.Available)
	if errors.Is(err, sql.ErrNoRows) {
		// QueryRowContext 把错误推迟到 Scan。只有“确实没查到”才能映射为商品不存在。
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

// change 共用执行和错误分类。statement 只来自本文件常量，用户输入始终通过占位符传递。
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
		// errors.Join 同时保留领域分类与原始原因。errors.Is 能沿两条分支查找。
		return errors.Join(ErrUnavailable, err)
	}
	return err
}
