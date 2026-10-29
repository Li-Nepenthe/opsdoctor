// 本文件：把系统说明书、索引账号和运维入口绑定到审批后端。
//
// 医生了解允许表的唯一来源是架构组件表。索引账号和只读账号必须连到同一个库，
// 否则“检查一个库，修改另一个库”会让审批失去意义。
//
// 主要内容：bindActions 核对库和表；Actions 返回固定后端；opsConfig 描述接入字段。
// 阅读建议：没有写账号和运维入口时，系统仍可以只读运行。

package target

import (
	"fmt"
	"strings"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/approval"
	"github.com/go-sql-driver/mysql"
)

// opsConfig 只保存部署者给的根地址和变量名，模型没有修改这些字段的入口。
type opsConfig struct {
	TokenEnv    string            `yaml:"token_env"`
	Services    map[string]string `yaml:"services"`
	PoolMin     int               `yaml:"pool_min"`
	PoolMax     int               `yaml:"pool_max"`
	WarmService string            `yaml:"warm_cache_service"`
}

// Actions 不导出账号，只把已经绑定当前系统的后端交给审批服务。
func (system *System) Actions() *approval.Backend { return system.actions }

// bindActions 必须在只读原生来源装配后调用，审批元数据复用原来的三连接池。
func (system *System) bindActions(config Config, values map[string]string) error {
	tables := []string{}
	dsn := values[config.MySQLDDLEnv]
	if dsn != "" {
		read, err := mysql.ParseDSN(values[config.MySQLDSNEnv])
		if err != nil {
			return fmt.Errorf("索引操作缺少有效只读来源")
		}
		write, err := mysql.ParseDSN(dsn)
		if err != nil || read.Addr != write.Addr || read.Net != write.Net || read.DBName != write.DBName {
			return fmt.Errorf("索引账号与只读账号必须使用同一主机和库")
		}
		// 允许表来自完整架构清单，不从故障场景或模型输入推断。
		prefix := "mysql:" + read.DBName + "."
		for _, component := range system.components {
			if strings.HasPrefix(component, prefix) {
				tables = append(tables, strings.TrimPrefix(component, prefix))
			}
		}
	}
	timeout := 2 * time.Minute
	if config.WriteTimeout != "" {
		var err error
		timeout, err = time.ParseDuration(config.WriteTimeout)
		if err != nil {
			return fmt.Errorf("write_timeout 格式错误")
		}
	}
	backend, err := approval.NewBackend(dsn, system.mysqlMetadata, tables, approval.OpsConfig{Services: config.Ops.Services, Token: values[config.Ops.TokenEnv], PoolMin: config.Ops.PoolMin, PoolMax: config.Ops.PoolMax, WarmService: config.Ops.WarmService}, timeout)
	if err != nil {
		return err
	}
	system.actions = backend
	system.closers = append(system.closers, backend.Close)
	return nil
}
