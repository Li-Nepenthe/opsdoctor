// 本文件：显式启用后，对真实 OpenSearch 验证服务和关键词查询。
//
// 默认没有 TEST_OPENSEARCH_URL 时跳过。地址、索引、五字段映射、服务和关键词由
// 操作者填写，测试不会猜索引、创建数据或清理数据。启用安全插件时仅提供只读账号。

package opensearch

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/source"
)

// TestIntegrationSearchLogs 只读取既有日志；测试通过不代表其他索引或采集链路已被验证。
func TestIntegrationSearchLogs(t *testing.T) {
	// 没有明确的接入参数就跳过，防止普通单元测试顺便访问真实系统。
	address := os.Getenv("TEST_OPENSEARCH_URL")
	if address == "" {
		t.Skip("真实日志测试需显式设置 TEST_OPENSEARCH_URL、INDEX、FIELDS(JSON)、SERVICE、KEYWORD；账号可用 USERNAME/PASSWORD（均加 TEST_OPENSEARCH_ 前缀）")
	}
	config := Config{URL: address, Index: os.Getenv("TEST_OPENSEARCH_INDEX"), Username: os.Getenv("TEST_OPENSEARCH_USERNAME"), Password: os.Getenv("TEST_OPENSEARCH_PASSWORD")}
	if err := json.Unmarshal([]byte(os.Getenv("TEST_OPENSEARCH_FIELDS")), &config.Fields); err != nil {
		t.Fatal("TEST_OPENSEARCH_FIELDS 必须是包含五字段映射的 JSON")
	}
	service, keyword := os.Getenv("TEST_OPENSEARCH_SERVICE"), os.Getenv("TEST_OPENSEARCH_KEYWORD")
	if service == "" || keyword == "" {
		t.Fatal("TEST_OPENSEARCH_SERVICE 和 TEST_OPENSEARCH_KEYWORD 必须明确填写")
	}
	client, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	end := time.Now().UTC()
	query := source.LogQuery{Service: service, Start: end.Add(-time.Hour), End: end, Limit: 10}
	rows, err := client.Search(t.Context(), query)
	if err != nil || len(rows) == 0 {
		t.Fatalf("按服务没有取得日志，条数=%d，错误=%v", len(rows), err)
	}
	t.Logf("按服务查询返回 %d 条日志", len(rows))
	query.Keyword = keyword
	rows, err = client.Search(t.Context(), query)
	if err != nil || len(rows) == 0 {
		t.Fatalf("按服务和关键词没有取得日志，条数=%d，错误=%v", len(rows), err)
	}
	t.Logf("按服务和关键词查询返回 %d 条日志", len(rows))
	// 显式提供级别时再验证统一接口的大写输入；真实源可能保留小写 severity.text。
	if level := os.Getenv("TEST_OPENSEARCH_LEVEL"); level != "" {
		query.Level = level
		rows, err = client.Search(t.Context(), query)
		if err != nil || len(rows) == 0 {
			t.Fatalf("按服务、关键词和级别没有取得日志，条数=%d，错误=%v", len(rows), err)
		}
		t.Logf("增加级别 %s 后返回 %d 条日志", level, len(rows))
	}
}
