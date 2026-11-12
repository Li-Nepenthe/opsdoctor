// 本文件：为每个系统绑定索引账号和运维服务的固定根地址。运维请求禁用重定向，以免令牌被带到另一台主机。
// 索引账号最多占一条连接，读取元数据仍用原有的 diag_ro 连接池。

package approval

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// OpsConfig 来自接入配置，不接受模型的同名字段覆盖。
type OpsConfig struct {
	Services    map[string]string
	Token       string // 只进 Authorization，不进审批单或日志。
	PoolMin     int    // 默认五条。
	PoolMax     int    // 默认两百条。
	WarmService string // 只有提供商品缓存的那个服务可以预热。
}

// Backend 只保存本系统的资源，不包含目录或其他系统的连接。
type Backend struct {
	metadata Metadata
	ddl      *sql.DB
	tables   map[string]bool
	ops      OpsConfig
	http     *http.Client
	timeout  time.Duration
}

// NewBackend 校验服务范围和索引账号后建立连接池，第一次使用时才连接数据库。
func NewBackend(dsn string, metadata Metadata, tables []string, ops OpsConfig, timeout time.Duration) (*Backend, error) {
	if timeout < 30*time.Second || timeout > 3*time.Minute {
		return nil, fmt.Errorf("写工具时限必须在 30s–3m 内")
	}
	if err := ops.applyDefaultsAndValidate(); err != nil {
		return nil, err
	}
	backend := &Backend{
		metadata: metadata, tables: map[string]bool{}, ops: ops, timeout: timeout,
		http: &http.Client{
			Timeout:       timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	for _, table := range tables {
		if !identifier(table) {
			return nil, fmt.Errorf("说明书中的表名不合法")
		}
		backend.tables[table] = true
	}
	if dsn != "" {
		if err := backend.openIndexPool(dsn); err != nil {
			return nil, err
		}
	}
	return backend, nil
}

func (ops *OpsConfig) applyDefaultsAndValidate() error {
	if ops.PoolMin == 0 {
		ops.PoolMin = 5
	}
	if ops.PoolMax == 0 {
		ops.PoolMax = 200
	}
	if ops.PoolMin < 5 || ops.PoolMax > 200 || ops.PoolMax < ops.PoolMin {
		return fmt.Errorf("连接池范围必须位于 5–200 内")
	}
	if len(ops.Services) > 0 && len(ops.Token) < 32 {
		return fmt.Errorf("运维入口需要独立的至少 32 字节令牌")
	}
	for service, address := range ops.Services {
		parsed, err := url.Parse(address)
		if !identifier(strings.ReplaceAll(service, "-", "_")) || err != nil || parsed.Host == "" ||
			parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
			(parsed.Path != "" && parsed.Path != "/") || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("运维服务名或固定根地址不合法")
		}
	}
	if ops.WarmService != "" && ops.Services[ops.WarmService] == "" {
		return fmt.Errorf("预热服务未在运维服务清单中")
	}
	return nil
}

// openIndexPool 单独建立索引账号的连接池，元数据仍由只读账号取得。
func (backend *Backend) openIndexPool(dsn string) error {
	config, err := mysql.ParseDSN(dsn)
	if err != nil || config.DBName == "" || config.Passwd == "" || config.User == "" ||
		config.User == "root" || config.User == "app" || config.User == "diag_ro" || backend.metadata == nil || len(backend.tables) == 0 {
		return fmt.Errorf("索引操作必须配置独立 INDEX 账号、元数据来源和允许表")
	}
	// 部署者误写多语句或文件选项，也不能让工具取得额外执行能力。
	config.MultiStatements = false
	config.AllowAllFiles = false
	config.InterpolateParams = false
	config.Timeout = 5 * time.Second
	config.ReadTimeout = backend.timeout
	config.WriteTimeout = 5 * time.Second
	config.Params = map[string]string{"lock_wait_timeout": "5"}
	connector, err := mysql.NewConnector(config)
	if err != nil {
		return fmt.Errorf("无法创建索引连接器")
	}
	backend.ddl = sql.OpenDB(connector)
	backend.ddl.SetMaxOpenConns(1)
	backend.ddl.SetMaxIdleConns(0)
	return nil
}

// Close 由接入目录统一调用，单次写工具不能拆掉共享后端。
func (backend *Backend) Close() error {
	backend.http.CloseIdleConnections()
	if backend.ddl != nil {
		return backend.ddl.Close()
	}
	return nil
}

// opsRequest 的路径由代码固定，返回体只接受小型运维结构，不读取商品内容。
func (backend *Backend) opsRequest(ctx context.Context, service, method, path string, input, output any) error {
	address := backend.ops.Services[service]
	if address == "" {
		return fmt.Errorf("运维服务未配置")
	}
	var raw []byte
	if input != nil {
		var err error
		raw, err = json.Marshal(input)
		if err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(address, "/")+path, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("无法创建运维请求")
	}
	request.Header.Set("Authorization", "Bearer "+backend.ops.Token)
	request.Header.Set("Content-Type", "application/json")
	response, err := backend.http.Do(request)
	if err != nil {
		return fmt.Errorf("运维请求未完成")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("运维接口返回状态 %d", response.StatusCode)
	}
	// 运维回复只包含少量计数，过大的回复可能是接错了地址，不能继续信任。
	data, err := io.ReadAll(io.LimitReader(response.Body, 8193))
	if err != nil || len(data) > 8192 {
		return fmt.Errorf("运维结果超限或未读完")
	}
	if json.Unmarshal(data, output) != nil {
		return fmt.Errorf("运维结果不是约定结构")
	}
	return nil
}

// databaseProblem 只保留错误号；DDL 的原始错误可能夹带库名、账号或连接信息。
func databaseProblem(err error) string {
	var problem *mysql.MySQLError
	if errors.As(err, &problem) {
		return fmt.Sprintf("MySQL 索引操作失败，错误号 %d", problem.Number)
	}
	return "索引操作没有收到确定结果，请按审批单核对真实状态；取消请求不保证 MySQL 已停止 DDL"
}
