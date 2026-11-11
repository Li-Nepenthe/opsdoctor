// 本文件：审批准备和恢复共用 diag_ro 连接池，检查列名、索引结构和旧 DDL 连接。这些检查只供程序使用，不在模型工具菜单中开放。

package mysqlinspect

import (
	"context"
	"fmt"
)

// Columns 使用绑定参数查固定库的列名，不能通过表名跨库。
func (client *Client) Columns(ctx context.Context, table string) ([]string, error) {
	if !tableName(table) {
		return nil, fmt.Errorf("表名不合法")
	}
	data, err := client.query(ctx, "SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=? AND TABLE_NAME=? ORDER BY ORDINAL_POSITION", 256, client.schema, table)
	if err != nil {
		return nil, err
	}
	columns := []string{}
	for _, row := range data.Rows {
		columns = append(columns, row[0])
	}
	return columns, nil
}

// IndexShape 比较完整列顺序和唯一性，不把碰巧同名的另一种索引当作执行成功。
func (client *Client) IndexShape(ctx context.Context, table, index string) ([]string, bool, bool, error) {
	if !tableName(table) || !tableName(index) {
		return nil, false, false, fmt.Errorf("表或索引名不合法")
	}
	data, err := client.query(ctx, "SELECT COLUMN_NAME,NON_UNIQUE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=? AND TABLE_NAME=? AND INDEX_NAME=? ORDER BY SEQ_IN_INDEX", 64, client.schema, table, index)
	if err != nil {
		return nil, false, false, err
	}
	if len(data.Rows) == 0 {
		return nil, false, false, nil
	}
	columns := []string{}
	for _, row := range data.Rows {
		columns = append(columns, row[0])
	}
	return columns, data.Rows[0][1] == "0", true, nil
}

// ConnectionAlive 只返回线程是否存在，不读取当前 SQL 文本或锁里的业务值。
func (client *Client) ConnectionAlive(ctx context.Context, id int64) (bool, error) {
	if id <= 0 {
		return false, fmt.Errorf("连接编号不合法")
	}
	data, err := client.query(ctx, "SELECT PROCESSLIST_ID FROM performance_schema.threads WHERE PROCESSLIST_ID=? LIMIT 1", 1, id)
	return len(data.Rows) > 0, err
}
