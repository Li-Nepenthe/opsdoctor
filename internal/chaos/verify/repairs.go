// 本文件：阻止把医生修过的环境误当成下一场实验的原始基线。
//
// 多留一个索引就可能让原来的慢查询不再出现。连接池在运行时被调整过，也会改变
// 排队症状。因此 healthy 不替人撤销，只说明残留了什么以及怎样恢复。
//
// 主要内容：WithRepairs 加入真实状态核对；repairIndexes 读全部架构表的索引；checkRepairs 检查池修改指标。
// 阅读建议：索引核对使用只读账号，绝不会为了让验收通过而自动 DROP INDEX。

package verify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/platform/mysqlx"
)

// WithRepairs 与旧参数检查串联；生产 healthy 和 selftest 都必须装配这条检查。
func (baseline *Baseline) WithRepairs(dsn string) *Baseline {
	baseline.indexes = func(ctx context.Context) ([]string, error) { return repairIndexes(ctx, dsn) }
	return baseline
}

// repairIndexes 先确认能看见五张表，权限缺失时不能把“不知道”说成“没有残留索引”。
func repairIndexes(parent context.Context, dsn string) ([]string, error) {
	if dsn == "" {
		return nil, fmt.Errorf("缺少 CHAOS_BASELINE_MYSQL_DSN，请设置指向 shop 的只读账号")
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	database, err := mysqlx.Open(ctx, mysqlx.Config{DSN: dsn, MaxOpenConns: 1, MaxIdleConns: 0, ConnMaxLifetime: time.Minute})
	if err != nil {
		return nil, fmt.Errorf("无法连接基线检查数据库，请确认只读账号与地址")
	}
	defer database.Close()
	// 信息模式只展示当前账号有权看见的表，因此先证明视野完整，再判断没有残留。
	var tables int
	if err = database.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA='shop' AND TABLE_NAME IN ('products','stock','orders','product_sales','processed_events') AND TABLE_TYPE='BASE TABLE'").Scan(&tables); err != nil || tables != 5 {
		return nil, fmt.Errorf("无法完整读取 shop 五张架构表，不能确认索引基线")
	}
	rows, err := database.QueryContext(ctx, "SELECT DISTINCT TABLE_NAME,INDEX_NAME FROM information_schema.STATISTICS WHERE TABLE_SCHEMA='shop' AND LEFT(INDEX_NAME,11)='idx_doctor_' ORDER BY TABLE_NAME,INDEX_NAME")
	if err != nil {
		return nil, fmt.Errorf("读取医生残留索引失败")
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var table, index string
		if err = rows.Scan(&table, &index); err != nil {
			return nil, err
		}
		names = append(names, table+"."+index)
	}
	return names, rows.Err()
}

// checkRepairs 既要指标值为零，也要三个进程都报告了值，不能把缺采集误当未修改。
func (baseline *Baseline) checkRepairs(ctx context.Context) error {
	if baseline.indexes == nil {
		return nil
	}
	names, err := baseline.indexes(ctx)
	if err != nil {
		return err
	}
	if len(names) > 0 {
		return fmt.Errorf("发现医生残留索引 %s。请由人按审批单的 DROP INDEX 撤销方法处理，再运行 chaos restore 和 healthy；本命令不会自动删除", strings.Join(names, ","))
	}
	// 缺少一条采集也会使条件为假，不让监控缺失掩盖被修改的连接池。
	clean, err := baseline.Query(ctx, `count(db_pool_runtime_changed{service=~"order-service|inventory-service|order-worker"}) == 3 and sum(db_pool_runtime_changed{service=~"order-service|inventory-service|order-worker"}) == 0`)
	if err != nil {
		return err
	}
	if !clean {
		return fmt.Errorf("连接池基线未确认：运行时被修改过，或三个进程的 db_pool_runtime_changed 指标未齐。请按审批单重启被修改服务恢复启动配置，并等待一次 Prometheus 抓取后重试")
	}
	return nil
}
