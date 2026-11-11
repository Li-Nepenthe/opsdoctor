// 本文件：从真实的 Prometheus 注册表采样，核对各指标的名字、类型、标签和说明。

package telemetry

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// TestMetricFamiliesGolden 先为每个指标向量制造一条样本，否则 Gather 看不到还未使用的向量。
func TestMetricFamiliesGolden(t *testing.T) {
	m := NewMetrics("sample")
	m.HTTP.WithLabelValues("GET", "/health", "200").Observe(0)
	m.RPCServer.WithLabelValues("Get", "OK").Observe(0)
	m.RPCClient.WithLabelValues("inventory", "Get", "OK").Observe(0)
	m.Retries.WithLabelValues("inventory", "Get").Inc()
	m.RateRejected.WithLabelValues("/orders").Inc()
	m.Idempotency.WithLabelValues("new").Inc()
	m.CacheRequests.WithLabelValues("product", "hit").Inc()
	m.CacheLoads.WithLabelValues("product").Inc()
	m.CacheConcurrentLoads.WithLabelValues("product").Inc()
	m.Produce.WithLabelValues("order.created", "success").Inc()
	m.Messages.WithLabelValues("order.created", "success").Inc()
	m.ProcessDuration.WithLabelValues("order.created").Observe(0)
	m.Loadgen.WithLabelValues("create", "success").Inc()
	// DBStats 只读连接池统计，无需连接数据库。
	if err := m.Database(new(sql.DB), "shop"); err != nil {
		t.Fatal(err)
	}
	families, err := m.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var got strings.Builder
	for _, family := range families {
		var labels []string
		for _, label := range family.Metric[0].Label {
			labels = append(labels, label.GetName())
		}
		fmt.Fprintf(&got, "%s\t%s\t%s\t%s\n", family.GetName(), family.GetType(), strings.Join(labels, ","), family.GetHelp())
	}
	want := metricCatalogForHost(t)
	if got.String() != want {
		t.Fatalf("采集到的指标：\n%s期望：\n%s", got.String(), want)
	}
}

// metricCatalogForHost 只挑当前系统能提供的 process 指标；Windows 没有 Linux 的 /proc 网络计数。
func metricCatalogForHost(t *testing.T) string {
	t.Helper()
	probe := prometheus.NewRegistry()
	probe.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	families, err := probe.Gather()
	if err != nil {
		t.Fatal(err)
	}
	available := make(map[string]bool)
	for _, family := range families {
		available[family.GetName()] = true
	}
	catalog, err := os.ReadFile("testdata/metric_families.golden.tsv")
	if err != nil {
		t.Fatal(err)
	}
	var want strings.Builder
	for row := range strings.SplitAfterSeq(string(catalog), "\n") {
		// 操作系统只决定该不该有这一行。类型、标签名、Help 仍从同一份 golden 读取。
		name, _, _ := strings.Cut(row, "\t")
		if !strings.HasPrefix(name, "process_") || available[name] {
			want.WriteString(row)
		}
	}
	return want.String()
}
