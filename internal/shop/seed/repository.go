// 本文件：批量造数的五表参数和事务写入。
//
// Run 把商品编号范围交给 insertBatch，它用 makeBatches 生成对应数据，再用同一个事务写入。
// 先看 inspect 的基线检查，再看每商品四单如何对应销量和去重记录。最后看占位符与回滚，不能拼接实际业务值。
//
// 主要内容：
//   - inspect：拒绝混杂或缺失的基线。这里只校验行数及连续编号，不是对生产数据的完整一致性审计。
//   - makeBatches：只在内存生成样本，方便单元测试检查 ID、金额和五表之间的对应关系。
//   - insertBatch：在一个事务里写五张表。result 是具名返回值，defer 可将回滚错误追加到原始错误。
//   - insertStatement：为非空且等宽的内部数据构造 VALUES。所有实际值都由驱动参数化传输。

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

// batch 保存一张表一批的参数。二维切片的每一项是一行，any 允许整数、字符串、时间混合。
type batch struct {
	statement string  // statement 是本文件中的固定 INSERT 前缀。
	rows      [][]any // rows 顺序与 INSERT 列顺序相同，执行时展开为占位符参数。
}

// makeBatches 只在内存生成样本，方便单元测试检查 ID、金额和五表之间的对应关系。
func makeBatches(first, last int64, now time.Time) []batch {
	products := batch{statement: productsInsert}
	stock := batch{statement: stockInsert}
	orders := batch{statement: ordersInsert}
	sales := batch{statement: salesInsert}
	processed := batch{statement: processedInsert}
	for skuID := first; skuID <= last; skuID++ {
		products.rows = append(products.rows, []any{skuID, fmt.Sprintf("商品%d", skuID), priceCents, now})
		stock.rows = append(stock.rows, []any{skuID, initialStock, now})
		// 历史订单视为已经统计过，直接写入销量和去重记录，避免阶段 2 的检查误报基线缺口。
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

// insertBatch 在一个事务里写五张表。result 是具名返回值，defer 可将回滚错误追加到原始错误。
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
		// append 的 ... 把一行切片展开。SQL 参数按“第一行全部列、第二行全部列”的顺序排列。
		placeholders[i] = rowPlaceholder
		arguments = append(arguments, row...)
	}
	return data.statement + strings.Join(placeholders, ","), arguments
}
