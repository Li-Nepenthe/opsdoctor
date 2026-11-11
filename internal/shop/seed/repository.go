// 本文件：按连续商品编号生成五张表的样本，每批在同一事务写完，供中断后继续造数。
// Run 调用 insertBatch，由它生成并写入对应批次的样本。

package seed

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// inspectSQL 只在离线初始化时扫描计数，利用各表主键读取行数/最大值，不给业务行加排他锁。
// 对正整数主键，COUNT=MAX 意味着从 1 开始连续。结合五表计数判断是否是完整的种子前缀。
const inspectSQL = `
SELECT (SELECT COUNT(*) FROM products), (SELECT COALESCE(MAX(id), 0) FROM products),
       (SELECT COUNT(*) FROM stock), (SELECT COUNT(*) FROM orders),
       (SELECT COALESCE(MAX(id), 0) FROM orders), (SELECT COUNT(*) FROM product_sales),
       (SELECT COUNT(*) FROM processed_events)`

// 批量 INSERT 的列固定在代码中。后面只拼接占位符，不拼接业务输入或表名。
// 新行写入涉及主键/唯一键检查与插入意向锁。每批五张表保持相同写入顺序，减少锁冲突。
const (
	productsInsert  = `INSERT INTO products (id, name, price_cents, created_at) VALUES `
	stockInsert     = `INSERT INTO stock (sku_id, available, updated_at) VALUES `
	ordersInsert    = `INSERT INTO orders (id, user_id, sku_id, quantity, amount_cents, status, created_at) VALUES `
	salesInsert     = `INSERT INTO product_sales (sku_id, sold) VALUES `
	processedInsert = `INSERT INTO processed_events (order_id, processed_at) VALUES `
)

// inspect 拒绝混杂或缺失的基线。这里只校验行数及连续编号，不是对生产数据的完整一致性审计。
func inspect(ctx context.Context, database *sql.DB) (int64, error) {
	var products, maxProduct, stock, orders, maxOrder, sales, processed int64
	err := database.QueryRowContext(ctx, inspectSQL).Scan(&products, &maxProduct, &stock, &orders, &maxOrder, &sales, &processed)
	if err != nil {
		return 0, fmt.Errorf("检查已有种子数据：%w", err)
	}
	if products != maxProduct || stock != products || sales != products || orders != products*ordersPerProduct || orders != maxOrder || processed != orders {
		// 不用 INSERT IGNORE 掩盖一半成功的旧数据，更不能自动清空用户已经操作过的库。
		return 0, fmt.Errorf("已有数据不符合种子基线，未写入；请使用独立空库或检查五张表的数据来源")
	}
	return products, nil
}

// batch 按固定 INSERT 的列序保存每行参数。
type batch struct {
	statement string
	rows      [][]any
}

// makeBatches 为同一批商品生成对应的库存、订单、销量和去重凭据。
func makeBatches(first, last int64, now time.Time) []batch {
	products := batch{statement: productsInsert}
	stock := batch{statement: stockInsert}
	orders := batch{statement: ordersInsert}
	sales := batch{statement: salesInsert}
	processed := batch{statement: processedInsert}
	for skuID := first; skuID <= last; skuID++ {
		products.rows = append(products.rows, []any{skuID, fmt.Sprintf("商品%d", skuID), priceCents, now})
		stock.rows = append(stock.rows, []any{skuID, initialStock, now})
		// 历史订单不会发送消息，须同时写入销量和去重记录，才能按订单件数核对。
		sales.rows = append(sales.rows, []any{skuID, ordersPerProduct})
		for offset := int64(1); offset <= ordersPerProduct; offset++ {
			orderID := (skuID-1)*ordersPerProduct + offset
			userID := 1001 + orderID%10_000 // 分布到一万个演示用户，给后续用户订单索引实验提供数据。
			orders.rows = append(orders.rows, []any{orderID, userID, skuID, 1, priceCents, "created", now})
			processed.rows = append(processed.rows, []any{orderID, now})
		}
	}
	return []batch{products, stock, orders, sales, processed}
}

// insertBatch 在同一事务写入五张表，任何一张失败都不能留下半批数据。
func insertBatch(ctx context.Context, database *sql.DB, first, last int64) (result error) {
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始种子事务：%w", err)
	}
	defer func() {
		// Commit 成功或 context 已自动回滚时得到 ErrTxDone，这是预期情况。其他清理错误仍保留。
		if err := transaction.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			result = errors.Join(result, fmt.Errorf("回滚种子事务：%w", err))
		}
	}()
	for _, data := range makeBatches(first, last, time.Now().UTC()) {
		statement, arguments := insertStatement(data)
		if _, err := transaction.ExecContext(ctx, statement, arguments...); err != nil {
			return fmt.Errorf("批量插入种子数据：%w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("提交种子事务：%w", err)
	}
	return nil
}

// insertStatement 为非空且等宽的内部数据构造 VALUES。所有实际值都由驱动参数化传输。
func insertStatement(data batch) (string, []any) {
	columns := len(data.rows[0])
	rowPlaceholder := "(" + strings.TrimSuffix(strings.Repeat("?,", columns), ",") + ")"
	placeholders := make([]string, len(data.rows))
	arguments := make([]any, 0, len(data.rows)*columns)
	for i, row := range data.rows {
		placeholders[i] = rowPlaceholder
		arguments = append(arguments, row...)
	}
	return data.statement + strings.Join(placeholders, ","), arguments
}
