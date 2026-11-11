// 本文件：订单号去重凭据与销量增量在同一事务提交，避免一边成功、另一边失败。
// 从 Apply 的重复键分支可以看到消费者重投时为何不再累加。

package sales

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/shop/events"
	"github.com/go-sql-driver/mysql"
)

// insertProcessedSQL 用 order_id 主键竞争处理权。重复消息会在唯一键处等待或得到 1062。
const insertProcessedSQL = `INSERT INTO processed_events (order_id, processed_at) VALUES (?, ?)`

// addSalesSQL 按 sku_id 主键更新一行并加排他锁。第一次出现的商品会直接插入。
// 同一商品的并发消息在数据库里串行加法，不使用“先读再写”覆盖其他请求的结果。
const addSalesSQL = `INSERT INTO product_sales (sku_id, sold) VALUES (?, ?) ON DUPLICATE KEY UPDATE sold = sold + ?`

// checkSalesSQL 的两个子查询属于同一条一致性读，共用快照。它不加排他行锁，但需要扫描两张表。
// SUM 汇总全表，没有能跳过所有明细的索引。这个检查由命令手动执行，不放到每次请求里。
const checkSalesSQL = `SELECT (SELECT COALESCE(SUM(quantity), 0) FROM orders), (SELECT COALESCE(SUM(sold), 0) FROM product_sales)`

// MySQLRepository 复用服务级连接池。事务期间会独占借出的那条连接。
type MySQLRepository struct{ database *sql.DB }

// NewMySQLRepository 不执行查询，启动时的探活和迁移由 app 完成。
func NewMySQLRepository(database *sql.DB) *MySQLRepository {
	return &MySQLRepository{database: database}
}

// Apply 返回此次是否遇到重复订单。
func (repository *MySQLRepository) Apply(ctx context.Context, event events.OrderCreated) (duplicate bool, result error) {
	tx, err := repository.database.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("开始销量事务：%w", err)
	}
	defer func() {
		// 成功 Commit 后 Rollback 返回 ErrTxDone，这是正常的清理路径。
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			// 回滚失败时也保留最初的写入错误。
			result = errors.Join(result, fmt.Errorf("回滚销量事务：%w", err))
		}
	}()
	_, err = tx.ExecContext(ctx, insertProcessedSQL, event.OrderID, time.Now().UTC())
	if err != nil {
		var mysqlError *mysql.MySQLError
		if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
			// 已有凭据说明此前事务已提交。本次不再加销量，回滚这个空事务即可。
			return true, nil
		}
		return false, fmt.Errorf("登记已处理订单：%w", err)
	}
	if _, err := tx.ExecContext(ctx, addSalesSQL, event.SKUID, event.Quantity, event.Quantity); err != nil {
		return false, fmt.Errorf("累加商品销量：%w", err)
	}
	// 即使 Commit 的响应丢失，下一次也会通过主键去重，安全地重复处理。
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("提交销量事务：%w", err)
	}
	return false, nil
}

// Check 只查询总量，不修改任何订单或销量，也不会把差值自动修正成零。
func (repository *MySQLRepository) Check(ctx context.Context) (Summary, error) {
	var summary Summary
	if err := repository.database.QueryRowContext(ctx, checkSalesSQL).Scan(&summary.Ordered, &summary.Sold); err != nil {
		return Summary{}, fmt.Errorf("核对订单与销量：%w", err)
	}
	summary.Gap = summary.Ordered - summary.Sold
	return summary, nil
}
