package migrate

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"
)

// migration 是一份已加载的迁移，不导出，避免调用方绕过文件校验后直接执行任意片段。
type migration struct {
	name      string // name 是完整文件名，也是 schema_migrations 主键。
	statement string // statement 是一条 SQL；不拆分分号，避免破坏字符串和 SQL 注释。
	checksum  string // checksum 是原始文件的 SHA-256 十六进制值，检查已执行文件是否被改写。
}

// load 按文件系统字典序读取迁移；格式、重复序号、空文件或读取失败都会返回带文件名的错误。
// 只读取调用方提供的 fs.FS，不访问网络；返回值归调用方独占，可以并行加载不同文件系统。
func load(source fs.FS, directory string) ([]migration, error) {
	entries, err := fs.ReadDir(source, directory)
	if err != nil {
		return nil, fmt.Errorf("读取迁移目录 %s：%w", directory, err)
	}
	var migrations []migration
	versions := make(map[string]bool)
	// 四位数字前缀保证字典序就是执行顺序，避免 10 排在 2 前面。
	pattern := regexp.MustCompile(`^[0-9]{4}_[A-Za-z0-9_\-]+\.sql$`)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		name := entry.Name()
		if !pattern.MatchString(name) || len(name) > 255 {
			return nil, fmt.Errorf("迁移文件 %s 必须采用 NNNN_description.sql 命名", name)
		}
		version := name[:4]
		if versions[version] {
			return nil, fmt.Errorf("迁移序号 %s 重复", version)
		}
		versions[version] = true
		data, err := fs.ReadFile(source, path.Join(directory, name))
		if err != nil {
			return nil, fmt.Errorf("读取迁移 %s：%w", name, err)
		}
		if strings.TrimSpace(string(data)) == "" {
			return nil, fmt.Errorf("迁移 %s 不能为空", name)
		}
		migrations = append(migrations, migration{name: name, statement: string(data), checksum: fmt.Sprintf("%x", sha256.Sum256(data))})
	}
	return migrations, nil
}
