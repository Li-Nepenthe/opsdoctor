// 本文件：验证执行计划工具接受的是单条简单 SELECT，而不是任意 SQL。
//
// 这些测试不连接 MySQL，危险输入在获得数据库连接之前就应被拒绝。
//
// 主要内容：
//   - TestSelectStatement：合法查询与写入、多语句、ANALYZE、注释、变量、函数绕过。
//
// 阅读建议：看每条用例后的理由，重点不是大小写，而是语句是否可能做别的事情。

package source

import "testing"

// TestSelectStatement 不放宽输入去兼容所有 SQL；诊断工具宁可要求改写成更简单的查询。
func TestSelectStatement(t *testing.T) {
	for _, testcase := range []struct {
		query string // 待检查的输入。
		valid bool   // 是否属于允许的诊断查询。
	}{
		{"SELECT * FROM stock WHERE sku_id=1", true},                                        // S01 手动取证需要的形状。
		{" select * from orders where user_id=1 order by created_at desc limit 20; ", true}, // 大小写、空白和单个末尾分号无害。
		{"SELECT COUNT(*) FROM stock", true},                                                // 常见聚合可用于看计划，不实际返回业务行。
		{"SELECT * FROM stock WHERE sku_id IN (1,2)", true},                                 // IN 不是要调用任意函数。
		{"UPDATE stock SET available=0", false},                                             // 写语句没有机会到达数据库。
		{"EXPLAIN SELECT * FROM stock", false},                                              // 前缀必须由工具自己加。
		{"EXPLAIN ANALYZE SELECT * FROM stock", false},                                      // ANALYZE 会执行查询。
		{"ANALYZE TABLE stock", false},                                                      // 不能借分析名义修改统计信息。
		{"SELECT 1; DELETE FROM stock", false},                                              // 不接受第二条语句。
		{"SELECT /*!50000 SLEEP(3) */ 1", false},                                            // MySQL 版本注释可能被执行，不能忽略。
		{"SELECT 1 -- comment", false},                                                      // 简单规则不支持 SQL 注释。
		{"SELECT @x:=1", false},                                                             // 不让工具修改会话变量。
		{"SELECT * INTO OUTFILE '/tmp/data' FROM stock", false},                             // 禁止导出文件。
		{"SELECT * FROM stock FOR UPDATE", false},                                           // 诊断不需要主动加行锁。
		{"SELECT SLEEP(2)", false},                                                          // 不支持会消耗额外资源的函数。
		{"SELECT custom_function()", false},                                                 // 存储函数可能在优化阶段产生副作用。
		{"SELECT `custom_function`()", false},                                               // 引号不能绕过函数限制。
		{"SELECT shop.COUNT(*) FROM stock", false},                                          // 限定名称可能指向存储函数，不能借内建函数名绕过。
		{"SELECT 'unterminated", false},                                                     // 错误引号不能让后续检查失去边界。
	} {
		t.Run(testcase.query, func(t *testing.T) {
			_, err := SelectStatement(testcase.query)
			if (err == nil) != testcase.valid {
				t.Fatalf("valid=%t err=%v", testcase.valid, err)
			}
		})
	}
}
