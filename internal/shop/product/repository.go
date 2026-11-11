// 本文件：缓存未命中后，按商品主键向 MySQL 查询名称和价格。
// 商品服务回源时调用 Get，只有确实没有记录才返回商品不存在。

package product

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// getProductSQL 使用 products 的主键定位一行。普通 SELECT 不主动持有排他行锁。
const getProductSQL = `SELECT id, name, price_cents FROM products WHERE id = ?`

// MySQLRepository 借用订单服务的连接池。
type MySQLRepository struct{ database *sql.DB }

// NewMySQLRepository 把已有连接池用于商品查询。
func NewMySQLRepository(database *sql.DB) *MySQLRepository {
	return &MySQLRepository{database: database}
}

// Get 只在 sql.ErrNoRows 时返回不存在，其他错误保留给调用方判断。
func (repository *MySQLRepository) Get(ctx context.Context, id int64) (Product, error) {
	var value Product
	err := repository.database.QueryRowContext(ctx, getProductSQL, id).Scan(&value.ID, &value.Name, &value.PriceCents)
	if errors.Is(err, sql.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	if err != nil {
		return Product{}, fmt.Errorf("读取商品：%w", err)
	}
	return value, nil
}
