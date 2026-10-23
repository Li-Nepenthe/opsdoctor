package migrate

import (
	"testing"
	"testing/fstest"
)

// TestLoadAndHistory 验证排序、校验和和只追加规则；用内存文件系统，不需要数据库。
func TestLoadAndHistory(t *testing.T) {
	files := fstest.MapFS{
		"migrations/0002_second.sql": &fstest.MapFile{Data: []byte("SELECT 'a;b'")},
		"migrations/0001_first.sql":  &fstest.MapFile{Data: []byte("SELECT 1")},
	}
	items, err := load(files, "migrations")
	if err != nil || len(items) != 2 {
		t.Fatalf("加载：%v %v", items, err)
	}
	if items[0].name != "0001_first.sql" || items[1].statement != "SELECT 'a;b'" {
		t.Fatal("排序或 SQL 内容被破坏")
	}
	// 字符串内的分号必须原样交给数据库，不能用 strings.Split 切开。
	history := map[string]string{items[0].name: items[0].checksum}
	if err := verifyHistory(items, history); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []map[string]string{
		// 已执行文件的校验和发生变化，说明旧迁移被改写。
		{items[0].name: "changed"},
		// 数据库记得一个本地不存在的版本，说明旧迁移被删除。
		{"0000_removed.sql": "missing"},
		// 数据库已执行 0002，却没执行本地的 0001，不能再把 0001 插回过去的顺序。
		{items[1].name: items[1].checksum},
	} {
		if err := verifyHistory(items, invalid); err == nil {
			t.Fatal("接受了被改写、移除或插队的历史")
		}
	}
}

// TestLoadRejectsInvalidFiles 检查不合法命名、重复序号和空文件，保证失败发生在执行 SQL 之前。
func TestLoadRejectsInvalidFiles(t *testing.T) {
	for name, files := range map[string]fstest.MapFS{
		// 序号不足四位，字典序将不再可靠，加载时就要拒绝。
		"bad_name": {"m/1_bad.sql": &fstest.MapFile{Data: []byte("SELECT 1")}},
		// 描述不同也不能共享同一个序号，否则迁移顺序有歧义。
		"duplicate": {"m/0001_a.sql": &fstest.MapFile{Data: []byte("SELECT 1")}, "m/0001_b.sql": &fstest.MapFile{Data: []byte("SELECT 2")}},
		// 只有空白的文件不算一次有效迁移，不能为它写成功记录。
		"empty": {"m/0001_empty.sql": &fstest.MapFile{Data: []byte(" \n")}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := load(files, "m"); err == nil {
				t.Fatal("未拒绝错误的迁移文件")
			}
		})
	}
}
