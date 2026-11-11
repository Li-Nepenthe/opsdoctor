// 本文件：在申请数据库锁前检查迁移文件。
// 从 load 读起，它为每份 SQL 保留执行顺序和用于核对历史的校验和。

package migrate

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"
)

type migration struct {
	name      string // 完整文件名也是迁移历史的主键。
	statement string // 整份文件作为一条 SQL，分号可能出现在字符串或注释里。
	checksum  string
}

func load(source fs.FS, directory string) ([]migration, error) {
	entries, err := fs.ReadDir(source, directory)
	if err != nil {
		return nil, fmt.Errorf("读取迁移目录 %s：%w", directory, err)
	}
	var migrations []migration
	versions := make(map[string]bool)
	// 四位序号保证按文件名排序就是执行顺序。
	pattern := regexp.MustCompile(`^[0-9]{4}_[A-Za-z0-9_\-]+\.sql$`)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		name := entry.Name()
		if !pattern.MatchString(name) || len(name) > 255 {
			// 完整文件名必须能存进历史表的 version 列。
			return nil, fmt.Errorf("迁移文件 %s 必须采用 NNNN_description.sql 命名", name)
		}
		version := name[:4]
		if versions[version] {
			// 同一序号只能对应一次迁移，描述不同也不能共用。
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
		// 校验和覆盖原始字节，旧文件只改空格也会在核对历史时被发现。
		migrations = append(migrations, migration{name: name, statement: string(data), checksum: fmt.Sprintf("%x", sha256.Sum256(data))})
	}
	return migrations, nil
}
