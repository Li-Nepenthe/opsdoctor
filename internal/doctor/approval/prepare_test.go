// 本文件：用虚构架构和元数据检查写工具的参数边界。
//
// 模型提供的 JSON 即使语法正确，也不能跨表、跨系统、换地址或扩大操作量。
// 这些测试直接调用程序校验，不依赖提示词劝模型遵守规则。
//
// 主要内容：TestWriteArguments 检查越界与伪造字段；TestIndexRecovery 检查真实索引优先于重做。
// 阅读建议：假的 sql.DB 只用来表示“配置了能力”，测试不会执行它的联网方法。

package approval

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
)

// fakeMetadata 的列来自测试架构；可以单独模拟索引已存在而结果没有保存。
type fakeMetadata struct {
	exists, wrong bool
	reads         int
}

// Columns 提供固定架构，让参数测试不需要真实数据库。
func (*fakeMetadata) Columns(context.Context, string) ([]string, error) {
	return []string{"sku_id", "quantity", "id"}, nil
}

// IndexShape 区分同名同形与同名异形，后者不能当成已完成。
func (meta *fakeMetadata) IndexShape(context.Context, string, string) ([]string, bool, bool, error) {
	meta.reads++
	if meta.wrong {
		return []string{"id"}, false, true, nil
	}
	return []string{"sku_id"}, true, meta.exists, nil
}

// ConnectionAlive 默认没有旧连接，索引存在性仍由上面的替身决定。
func (*fakeMetadata) ConnectionAlive(context.Context, int64) (bool, error) { return false, nil }

// TestWriteArguments 每个拒绝理由都是独立边界，不能用缩小允许范围来掩盖缺失校验。
func TestWriteArguments(t *testing.T) {
	backend := &Backend{ddl: &sql.DB{}, metadata: &fakeMetadata{}, tables: map[string]bool{"stock": true}, ops: OpsConfig{PoolMin: 5, PoolMax: 200, Services: map[string]string{"inventory-service": "http://example.invalid"}, WarmService: "inventory-service"}}
	base := `"reason":"证据说明","evidence_steps":[7],"expected_effect":"减少查找范围","risk":"额外 IO"`
	// 每行的名字说明拒绝原因；索引申请都带齐必填字段，避免被更早的缺字段检查挡住。
	for _, test := range []struct{ name, tool, args string }{
		{"说明书以外的表", "create_index", `"table":"users","columns":["id"]`},
		{"超过三列", "create_index", `"table":"stock","columns":["id","sku_id","quantity","extra"]`},
		{"不存在的列", "create_index", `"table":"stock","columns":["secret"]`},
		{"同列重复", "create_index", `"table":"stock","columns":["sku_id","sku_id"]`},
		{"SQL 注入列名", "create_index", `"table":"stock","columns":["sku_id` + "`" + `;DROP TABLE stock"]`},
		{"伪造主机", "create_index", `"table":"stock","columns":["sku_id"],"host":"other"`},
		{"伪造系统", "create_index", `"table":"stock","columns":["sku_id"],"target":"other"`},
		{"任意 URL", "warm_cache", `"ids":[1],"url":"http://other"`},
		{"重复参数", "warm_cache", `"ids":[1],"ids":[2]`},
		{"连接池太小", "set_db_pool_size", `"service":"inventory-service","size":4`},
		{"连接池太大", "set_db_pool_size", `"service":"inventory-service","size":201`},
		{"不存在服务", "set_db_pool_size", `"service":"other","size":20`},
		{"商品负数", "warm_cache", `"ids":[-1]`},
		{"商品重复", "warm_cache", `"ids":[1,1]`},
	} {
		if test.tool == "create_index" {
			test.args += `,"unique":false`
		}
		t.Run(test.name, func(t *testing.T) {
			if _, err := backend.prepare(t.Context(), strings.Repeat("a", 32), test.tool, json.RawMessage("{"+base+","+test.args+"}"), map[int]bool{7: true}); err == nil {
				t.Fatal("越界参数被接受")
			}
		})
	}
	ids := make([]int, 101)
	for i := range ids {
		ids[i] = i + 1
	}
	rawIDs, _ := json.Marshal(ids)
	if _, err := backend.prepare(t.Context(), strings.Repeat("a", 32), "warm_cache", json.RawMessage("{"+base+`,"ids":`+string(rawIDs)+"}"), map[int]bool{7: true}); err == nil {
		t.Fatal("超过一百项")
	}
	good := json.RawMessage("{" + base + `,"table":"stock","columns":["sku_id"],"unique":true}`)
	if _, err := backend.prepare(t.Context(), strings.Repeat("a", 32), "create_index", good, map[int]bool{8: true}); err == nil {
		t.Fatal("编造证据")
	}
	record, err := backend.prepare(t.Context(), strings.Repeat("a", 32), "create_index", good, map[int]bool{7: true})
	if err != nil || !strings.Contains(record.Plan.Preview, "ALGORITHM=INPLACE LOCK=NONE") || !strings.Contains(record.Plan.Rollback, "DROP INDEX `idx_doctor_") {
		t.Fatalf("预览或撤销不完整：%v", err)
	}
}

// TestIndexRecovery 崩溃后的真实索引已经存在时，不再发送 DDL；不同形状也不会当成功。
func TestIndexRecovery(t *testing.T) {
	meta := &fakeMetadata{exists: true}
	backend := &Backend{ddl: &sql.DB{}, metadata: meta, tables: map[string]bool{"stock": true}}
	record := Record{Tool: "create_index", Status: "executing", Plan: Plan{Table: "stock", Index: "idx_doctor_test", Columns: []string{"sku_id"}, Unique: true}}
	// saveConnection 只有真的要写才会调用；此用例若重做会立即失败。
	data, err := backend.createIndex(t.Context(), record, func(int64) error { t.Fatal("已有索引仍重做"); return nil })
	if err != nil || data == nil || meta.reads != 1 {
		t.Fatal("没有核对已有索引")
	}
	meta.wrong = true
	if _, err = backend.createIndex(t.Context(), record, func(int64) error { return nil }); err == nil {
		t.Fatal("错误形状被当作成功")
	}
	record.Status = "pending"
	if got := backend.execute(t.Context(), record, nil); got.Status != "rejected" {
		t.Fatal("未经批准可以执行")
	}
}
