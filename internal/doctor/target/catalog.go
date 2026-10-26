// 本文件：为每个系统分别创建数据源和工具注册表。
//
// Catalog 相当于按系统名索引的通讯录。取出某个 System 后，它的工具就固定使用
// 这套连接；Run 的参数里没有系统名，也不能拿到连接字符串。
//
// 主要内容：
//   - Load：加载全部配置，逐个装配；失败时关闭已经创建的连接。
//   - Catalog、System：提供系统查找、令牌验证与公开工具说明。
//   - build：为单个系统绑定适配器，不使用包级共享客户端。
//
// 阅读建议：先看 System 的私有字段，再看 build 如何把不同来源接到 source 接口上。

package target

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source/jaeger"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source/loki"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source/mysqlinspect"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source/pprof"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source/prometheus"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source/redisinspect"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/tools"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

// System 不导出令牌与后端连接，接口层只能使用固定注册表。
type System struct {
	name     string          // 创建任务时绑定的名字，构造后不再更改。
	token    [32]byte        // 只保留令牌摘要，用定长比较检查推送身份。
	registry *tools.Registry // 已绑定这个系统的工具集合。
	closers  []func() error  // 进程结束或装配失败时归还连接资源。
}

// Catalog 不公开内部 map，调用方无法在运行中替换系统。
type Catalog struct{ systems map[string]*System }

// Name 返回稳定系统名，不把架构说明或凭据混到任务标识里。
func (system *System) Name() string { return system.name }

// Tools 返回已经固定数据源的注册表；注册表本身没有运行时增删接口。
func (system *System) Tools() *tools.Registry { return system.registry }

// Authenticate 对两个摘要作定长比较，错误令牌不会因为碰巧是另一个系统的令牌而通过。
func (system *System) Authenticate(token string) bool {
	candidate := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(system.token[:], candidate[:]) == 1
}

// Get 精确匹配系统名，不把不存在的名字退回默认 shop。
func (catalog *Catalog) Get(name string) (*System, bool) {
	value, ok := catalog.systems[name]
	return value, ok
}

// Names 排序后返回副本，CLI 输出不会随着 map 的遍历顺序变化。
func (catalog *Catalog) Names() []string {
	names := make([]string, 0, len(catalog.systems))
	for name := range catalog.systems {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Close 即使一条连接关闭失败，也继续关闭其他系统的连接。
func (catalog *Catalog) Close() error {
	var problems []error
	for _, system := range catalog.systems {
		for _, close := range system.closers {
			problems = append(problems, close())
		}
	}
	return errors.Join(problems...)
}

// Load 先检查所有凭据冲突，再创建任何客户端，防止装配一半才发现系统共用身份。
func Load(directory string, lookup func(string) (string, bool)) (*Catalog, error) {
	// 每个系统逐个装配。中途失败会关闭前面已建好的客户端，不能留下半套可用目录。
	configs, err := readConfigs(directory)
	if err != nil {
		return nil, err
	}
	values, err := credentials(configs, lookup)
	if err != nil {
		return nil, err
	}
	catalog := &Catalog{systems: map[string]*System{}}
	for _, config := range configs {
		profile, err := readFile(directory, config.Profile, 32<<10)
		if err != nil {
			_ = catalog.Close()
			return nil, err
		}
		if strings.TrimSpace(string(profile)) == "" {
			_ = catalog.Close()
			return nil, fmt.Errorf("系统 %s 的说明书为空", config.Name)
		}
		system, err := build(config, string(profile), values)
		if err != nil {
			_ = catalog.Close()
			return nil, fmt.Errorf("系统 %s：%w", config.Name, err)
		}
		catalog.systems[config.Name] = system
	}
	return catalog, nil
}

// credentials 除了变量名，还检查账号身份；换一个变量名不能把同一个账号重复接进来。
func credentials(configs []Config, lookup func(string) (string, bool)) (map[string]string, error) {
	// 先取得变量值，再比较账号身份。变量名称不同不代表实际账号不同。
	values, identities := map[string]string{}, map[string]string{}
	for _, config := range configs {
		for _, name := range config.variables() {
			if name == "" {
				continue
			}
			value, ok := lookup(name)
			if !ok || strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("系统 %s 缺少环境变量 %s", config.Name, name)
			}
			values[name] = value
			identity, err := credentialIdentity(config, name, value)
			if err != nil {
				return nil, err
			}
			if previous, exists := identities[identity]; exists {
				return nil, fmt.Errorf("系统 %s 与 %s 共用告警令牌或数据源账号", previous, config.Name)
			}
			identities[identity] = config.Name
		}
	}
	return values, nil
}

// credentialIdentity 不包含密码；只要主机和用户名一样，就已经是同一个账号。
func credentialIdentity(config Config, name, value string) (string, error) {
	// 令牌按完整值比较；数据库按地址和用户名比较，不能靠换密码绕过账号隔离。
	switch name {
	case config.AlertTokenEnv:
		if len(value) < 32 {
			return "", fmt.Errorf("%s 至少需要 32 字节", name)
		}
		return "token:" + value, nil
	case config.MySQLDSNEnv:
		parsed, err := drivermysql.ParseDSN(value)
		if err != nil {
			return "", fmt.Errorf("%s 不是合法 MySQL DSN", name)
		}
		return "mysql:" + parsed.Net + ":" + parsed.Addr + ":" + parsed.User, nil
	case config.RedisURLEnv:
		parsed, err := redis.ParseURL(value)
		if err != nil {
			return "", fmt.Errorf("%s 不是合法 Redis URL", name)
		}
		return "redis:" + parsed.Addr + ":" + parsed.Username, nil
	}
	return "", fmt.Errorf("未知凭据变量")
}

// build 把具体后端适配器装进接口；未配置的数据源不会生成对应工具。
func build(config Config, profile string, values map[string]string) (_ *System, result error) {
	system := &System{name: config.Name, token: sha256.Sum256([]byte(values[config.AlertTokenEnv]))}
	// 具名 result 让 defer 知道是否装配失败。失败时不能留下半套连接池。
	defer func() {
		if result != nil {
			for _, close := range system.closers {
				_ = close()
			}
		}
	}()
	deps := tools.Dependencies{Profile: profile}
	if err := system.bindHTTP(config, &deps); err != nil {
		return nil, err
	}
	if err := system.bindNative(config, values, &deps); err != nil {
		return nil, err
	}
	system.registry = tools.New(deps)
	return system, nil
}

// bindHTTP 装配指标、调用链和日志查询，关闭函数与客户端一起登记。
func (system *System) bindHTTP(config Config, deps *tools.Dependencies) error {
	// 每个客户端单独创建并登记关闭函数；一个系统不能借用另一个系统的连接。
	if config.Prometheus != "" {
		client, err := prometheus.New(config.Prometheus)
		if err != nil {
			return err
		}
		deps.Metrics = client
		system.closers = append(system.closers, func() error { client.Close(); return nil })
	}
	if config.Jaeger != "" {
		client, err := jaeger.New(config.Jaeger)
		if err != nil {
			return err
		}
		deps.Traces = client
		system.closers = append(system.closers, func() error { client.Close(); return nil })
	}
	if config.Loki != "" {
		client, err := loki.New(config.Loki)
		if err != nil {
			return err
		}
		deps.Logs = client
		system.closers = append(system.closers, func() error { client.Close(); return nil })
	}
	return nil
}

// bindNative 装配需要独立账号的数据库，以及配置里逐个列出的进程管理端口。
func (system *System) bindNative(config Config, values map[string]string, deps *tools.Dependencies) error {
	// 只在配置里明确提供了凭据或地址时建立连接，不给缺项补默认高权限账号。
	if config.MySQLDSNEnv != "" {
		client, err := mysqlinspect.New(values[config.MySQLDSNEnv])
		if err != nil {
			return err
		}
		deps.MySQL = client
		system.closers = append(system.closers, client.Close)
	}
	if config.RedisURLEnv != "" {
		client, err := redisinspect.New(values[config.RedisURLEnv])
		if err != nil {
			return err
		}
		deps.Redis = client
		system.closers = append(system.closers, client.Close)
	}
	if len(config.Pprof) > 0 {
		client, err := pprof.New(config.Pprof)
		if err != nil {
			return err
		}
		deps.Profiles = client
		system.closers = append(system.closers, func() error { client.Close(); return nil })
	}
	return nil
}
