// 本文件：读取并校验迁移文件，不执行 SQL。
//
// Apply 先通过 load 得到有顺序、有校验和的迁移清单，再开始碰数据库。
// 把文件规则独立出来，重复序号、空文件和命名错误都能离线验证。
// 先看 migration，再读 load 的筛选和排序。

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
// 接收 fs.FS 而不是磁盘目录，使生产的 embed 文件和测试的内存文件能走同一套规则。
func load(source fs.FS, directory string) ([]migration, error) {
	// 第 1 步：fs.ReadDir 返回按文件名排序的目录项，配合等宽序号决定执行顺序。
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
			// 迁移只取当前目录的 SQL 文件；说明文件和子目录不会被误当成迁移执行。
			continue
		}
		name := entry.Name()
		if !pattern.MatchString(name) || len(name) > 255 {
			// 先验证命名和历史表 version 字段的长度限制，再截取前四位，避免越界和写库失败。
			return nil, fmt.Errorf("迁移文件 %s 必须采用 NNNN_description.sql 命名", name)
		}
		version := name[:4]
		if versions[version] {
			// 序号相同但描述不同仍然冲突，不能依赖描述的字母顺序决定先执行谁。
			return nil, fmt.Errorf("迁移序号 %s 重复", version)
		}
		versions[version] = true
		// 第 2 步：fs.FS 的路径约定使用 /，所以用 path.Join，而不是依赖操作系统的 filepath.Join。
		data, err := fs.ReadFile(source, path.Join(directory, name))
		if err != nil {
			return nil, fmt.Errorf("读取迁移 %s：%w", name, err)
		}
		if strings.TrimSpace(string(data)) == "" {
			// 空白文件没有可执行内容，却可能占住一个迁移版本，加载阶段就明确拒绝。
			return nil, fmt.Errorf("迁移 %s 不能为空", name)
		}
		// 保留完整 SQL，并对原始字节计算校验和；连注释或空白变化也会被识别为旧文件被改写。
		migrations = append(migrations, migration{name: name, statement: string(data), checksum: fmt.Sprintf("%x", sha256.Sum256(data))})
	}
	return migrations, nil
}
