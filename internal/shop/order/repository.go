// 本文件：保存订单，并按主键或用户读取订单。
//
// service 扣库存成功后才调用 Create。这里的事务只覆盖 MySQL，不能同时提交远端库存或 Kafka。
// 单独放 SQL，方便核对列表是否使用了阶段 1 创建的联合索引。
//
// 主要内容：
//   - Create：插入订单并提交，保留“提交可能成功”的错误信息。
//   - Get：通过主键读取订单详情。
//   - List：使用用户与创建时间索引分页。
//   - scanOrder：共用字段扫描顺序，避免详情和列表的字段对错位置。
//
// 阅读建议：先看 Commit 失败为什么区别于 INSERT 失败，再看分页偏移量的限制。

package order

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// insertOrderSQL 新增一行，InnoDB 负责分配主键。只有事务提交后订单才对其他连接可见。
const insertOrderSQL = `INSERT INTO orders (user_id, sku_id, quantity, amount_cents, status, created_at) VALUES (?, ?, ?, ?, ?, ?)`

// getOrderSQL 通过主键定位一行。普通 SELECT 不主动持有排他行锁。
const getOrderSQL = `SELECT id, user_id, sku_id, quantity, amount_cents, status, created_at FROM orders WHERE id = ?`

// listOrdersSQL 用 idx_orders_user_created 先定位用户，再按时间倒序读取。相同时间用主键确定顺序。
// InnoDB 二级索引自带主键，仍需回表取金额等字段。偏移量很大时要跳过很多行，这是页码分页的已知代价。
const listOrdersSQL = `SELECT id, user_id, sku_id, quantity, amount_cents, status, created_at FROM orders WHERE user_id = ? ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`

// MySQLRepository 只借用连接池。一个写事务会借用一条连接直到提交或回滚。
type MySQLRepository struct{ database *sql.DB } // database 的关闭由 app 负责。

// NewMySQLRepository 让订单与商品查询复用服务级连接池，不为每个包创建新池。
func NewMySQLRepository(database *sql.DB) *MySQLRepository {
	return &MySQLRepository{database: database}
}

// Create 即使提交响应丢失也返回已分配的编号，人工核对可以按这个编号查数据库。
func (repository *MySQLRepository) Create(ctx context.Context, draft Order) (saved Order, result error) {
	saved = draft
	tx, err := repository.database.BeginTx(ctx, nil)
	if err != nil {
		return saved, &PersistenceError{Cause: fmt.Errorf("开始订单事务：%w", err)}
	}
	defer func() {
		// result 是具名返回值，defer 可以在返回前补充回滚失败原因。
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			result = errors.Join(result, fmt.Errorf("回滚订单事务：%w", err))
		}
	}()
	inserted, err := tx.ExecContext(ctx, insertOrderSQL, draft.UserID, draft.SKUID, draft.Quantity, draft.AmountCents, draft.Status, draft.CreatedAt)
	if err != nil {
		return saved, &PersistenceError{Cause: fmt.Errorf("写入订单：%w", err)}
	}
	saved.OrderID, err = inserted.LastInsertId()
	if err != nil {
		return saved, &PersistenceError{Cause: fmt.Errorf("读取订单编号：%w", err)}
	}
	if err := tx.Commit(); err != nil {
		// COMMIT 已经发出，网络断开时无法仅凭错误证明数据库没有提交。
		return saved, &PersistenceError{Cause: fmt.Errorf("提交订单：%w", err), MayHaveCommitted: true}
	}
	return saved, nil
}

// rowScanner 同时适用于 sql.Row 和 sql.Rows，复用扫描顺序但不创建通用仓储框架。
type rowScanner interface{ Scan(...any) error }

// scanOrder 按 SELECT 的固定列顺序填充模型，时间由连接配置保证为 UTC。
func scanOrder(row rowScanner) (Order, error) {
	var value Order
	err := row.Scan(&value.OrderID, &value.UserID, &value.SKUID, &value.Quantity, &value.AmountCents, &value.Status, &value.CreatedAt)
	return value, err
}

// Get 只有确实没有记录时返回 ErrNotFound，不把数据库故障伪装成订单不存在。
func (repository *MySQLRepository) Get(ctx context.Context, id int64) (Order, error) {
	value, err := scanOrder(repository.database.QueryRowContext(ctx, getOrderSQL, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("查询订单：%w", err)
	}
	return value, nil
}

// List 关闭 Rows 后连接才能及时回到池里。查询失败时不会返回半页结果假装成功。
func (repository *MySQLRepository) List(ctx context.Context, userID int64, page, pageSize int) (Page, error) {
	result := Page{Items: make([]Order, 0, pageSize), Page: page, PageSize: pageSize}
	rows, err := repository.database.QueryContext(ctx, listOrdersSQL, userID, pageSize, (page-1)*pageSize)
	if err != nil {
		return Page{}, fmt.Errorf("查询用户订单：%w", err)
	}
	defer rows.Close()
	for rows.Next() {
		value, err := scanOrder(rows)
		if err != nil {
			return Page{}, fmt.Errorf("读取用户订单：%w", err)
		}
		result.Items = append(result.Items, value)
	}
	// Next 返回 false 也可能是连接读取失败，必须再检查 Err。
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("遍历用户订单：%w", err)
	}
	return result, nil
}
