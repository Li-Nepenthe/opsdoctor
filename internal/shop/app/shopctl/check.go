// 本文件：提供订单件数与商品销量的只读核对命令。
//
// Run 把 check 子命令交给这里，再由 sales 仓储执行汇总查询。
// 这里不补发消息、不修改销量，也不自动把差值“修平”。
//
// 主要内容：
//   - runCheck：连接业务库，在有限时间内查询两个总量。
//   - writeCheck：显示真实差值，非零时说明可能原因并返回失败。
//
// 阅读建议：先停住新流量并等消费者追平，再用本命令判断最终总量。运行中出现短暂差值不等于丢消息。

package shopctl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/config"
	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/app"
	"github.com/Li-Nepenthe/opsdoctor/internal/shop/sales"
)

// runCheck 不执行迁移，保持核对命令只读。表不存在时应先按 README 启动服务或造数。
func runCheck(parent context.Context, arguments []string, reader config.Reader, output io.Writer) (result error) {
	if len(arguments) != 0 {
		return fmt.Errorf("check 不接受参数")
	}
	settings, err := app.DatabaseConfig(reader)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	database, err := mysqlx.Open(ctx, settings)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, database.Close()) }()
	// 读取的是同一个快照里的件数之和，连接池关闭前不修改任何业务数据。
	summary, err := sales.NewMySQLRepository(database).Check(ctx)
	if err != nil {
		return err
	}
	return writeCheck(output, summary)
}

// writeCheck 只报告事实。差值可能来自消费延迟，也可能来自已创建订单但消息发送失败。
func writeCheck(output io.Writer, summary sales.Summary) error {
	if _, err := fmt.Fprintf(output, "ordered_quantity=%d sold_quantity=%d gap=%d\n", summary.Ordered, summary.Sold, summary.Gap); err != nil {
		return err
	}
	if summary.Gap != 0 {
		return fmt.Errorf("总量尚未对齐：先停止新流量并等待消费追平；仍有差值时核对消息发送失败、补偿失败或人工数据变更。本命令不自动修复")
	}
	_, err := fmt.Fprintln(output, "两个总量一致；这不能单独证明每个商品都正确，重复与遗漏也可能在总量上抵消。")
	return err
}
