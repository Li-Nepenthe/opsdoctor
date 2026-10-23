// 本文件：在一个 MySQL 事务里完成订单去重和销量累加。
//
// processed_events 的主键相当于“这笔订单已经记过账”的凭据。
// 凭据与销量必须一起提交，否则可能出现只记凭据、没加销量的漏统计。
//
// 主要内容：
//   - MySQLRepository、NewMySQLRepository：借用 app 的数据库连接池。
//   - Apply：插入去重凭据，累加销量，然后提交事务。
//   - Check：在同一个数据库快照里读取订单总件数与已统计销量。
//
// 阅读建议：重点看重复键分支，以及 defer 为什么要区分事务已结束和回滚失败。

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
type MySQLRepository struct{ database *sql.DB } // database 的关闭由 app 负责。

// NewMySQLRepository 不执行查询，启动时的探活和迁移由 app 完成。
func NewMySQLRepository(database *sql.DB) *MySQLRepository {
	return &MySQLRepository{database: database}
}

// Apply 的 result 是具名返回值。defer 在函数返回前还能修改它，用来保留回滚失败的信息。
func (repository *MySQLRepository) Apply(ctx context.Context, event events.OrderCreated) (result error) {
	tx, err := repository.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始销量事务：%w", err)
	}
	defer func() {
		// 成功 Commit 后 Rollback 返回 ErrTxDone，这是正常的清理路径。
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			// Join 把两个原因保存在同一个 error 中，errors.Is 仍能匹配其中任意一个。
			result = errors.Join(result, fmt.Errorf("回滚销量事务：%w", err))
		}
	}()
	_, err = tx.ExecContext(ctx, insertProcessedSQL, event.OrderID, time.Now().UTC())
	if err != nil {
		var mysqlError *mysql.MySQLError
		if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
			// 已有凭据说明此前事务已提交。本次不再加销量，回滚这个空事务即可。
			return nil
		}
		return fmt.Errorf("登记已处理订单：%w", err)
	}
	if _, err := tx.ExecContext(ctx, addSalesSQL, event.SKUID, event.Quantity, event.Quantity); err != nil {
		return fmt.Errorf("累加商品销量：%w", err)
	}
	// 即使 Commit 的响应丢失，下一次也会通过主键去重，安全地重复处理。
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交销量事务：%w", err)
	}
	return nil
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
