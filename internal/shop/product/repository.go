// 本文件：按商品主键读取名称和价格。
//
// service 在缓存未命中时调用这里。把 SQL 放在仓储文件，便于检查索引是否符合访问方式。
//
// 主要内容：
//   - MySQLRepository、NewMySQLRepository：借用 app 管理的连接池。
//   - Get：读取单个商品，并把“没有记录”转换成业务错误。
//
// 阅读建议：注意查询只有一个主键条件，没有扫描商品列表。

package product

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// getProductSQL 使用 products 的主键定位一行。普通 SELECT 不主动持有排他行锁。
const getProductSQL = `SELECT id, name, price_cents FROM products WHERE id = ?`

// MySQLRepository 不创建连接池，也不关闭它。这些生命周期工作属于 app。
type MySQLRepository struct{ database *sql.DB } // database 会为查询借还连接。

// NewMySQLRepository 复用已有连接池，避免每个业务包重复占用数据库连接。
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
