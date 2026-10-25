// 本文件：定义一个可重复实验需要保存的输入与判断条件。
//
// 场景文件只由宿主机上的 chaos 读取。业务服务不导入本包，也不读取标准答案。
// 把动作写成固定字段，是为了拒绝任意 SQL 或 shell 命令。
//
// 主要内容：
//   - Scenario：组织注入、恢复、症状和调查参考答案。
//   - Step：描述一种受限制的外部操作。
//   - Symptom：规定观察表达式和最长等待时间。
//
// 阅读建议：先把一个 YAML 与这些字段对照，再读 load.go 的校验。

package scenario

// Scenario 把实验操作与验收条件放在一起，避免两者各自修改后不再对应。
type Scenario struct {
	ID          string   `yaml:"id"`                  // 文件名必须与编号相同，只接收阶段 4 的编号。
	Title       string   `yaml:"title"`               // 显示给操作者，不传给业务服务。
	Inject      []Step   `yaml:"inject"`              // 按顺序执行，任一步失败都需要恢复。
	Restore     []Step   `yaml:"restore"`             // 显式列出恢复顺序，数据库恢复尤其不能猜。
	Symptom     Symptom  `yaml:"symptom"`             // 用实际指标判断症状，不读取注入器自己的状态。
	GroundTruth Truth    `yaml:"ground_truth"`        // 将来给评测器使用，阶段 4 只校验非空。
	Evidence    []string `yaml:"evidence_milestones"` // 人类工程师应拿到的证据。
}

// Step 只允许已经实现的动作。加载器会拒绝未知字段，防止拼错名字后静默失效。
type Step struct {
	Action      string            `yaml:"action"`                 // drop_index、deploy、latency 等固定名称。
	Service     string            `yaml:"service,omitempty"`      // 允许操作的 Compose 服务。
	Environment map[string]string `yaml:"environment,omitempty"`  // 只接受不含秘密的配置白名单。
	Table       string            `yaml:"table,omitempty"`        // 索引动作只允许 stock 或 orders。
	Index       string            `yaml:"index,omitempty"`        // 只能操作计划书指定的两个索引。
	Proxy       string            `yaml:"proxy,omitempty"`        // 三条预先登记的链路之一。
	LatencyMS   int               `yaml:"latency_ms,omitempty"`   // 延迟作用于真实 TCP 数据，不伪造响应。
	Stream      string            `yaml:"stream,omitempty"`       // upstream 或 downstream。
	QPS         *int              `yaml:"qps,omitempty"`          // 指针区分省略和明确暂停。
	HotSKUID    *int64            `yaml:"hot_sku_id,omitempty"`   // 0 恢复正常选品，正数集中商品请求。
	FirstSKUID  int64             `yaml:"first_sku_id,omitempty"` // 缓存操作从哪个商品开始。
	Count       int               `yaml:"count,omitempty"`        // 最多处理一万个商品缓存，不扫描或清空幂等数据。
	TTLSeconds  int               `yaml:"ttl_seconds,omitempty"`  // 给商品缓存设置共同的到期时间。
	Image       string            `yaml:"image,omitempty"`        // 只允许两个事先构建的本项目订单镜像。
}

// Symptom 的表达式采用 PromQL，即 Prometheus 用来筛选和计算指标的查询语言。
type Symptom struct {
	PromQL string   `yaml:"promql"` // 使用不带 bool 的比较表达式；返回有限样本就表示条件成立。
	Within string   `yaml:"within"` // 必须为不超过三分钟的正时长。
	Alerts []string `yaml:"alerts"` // 至少响过其中一条症状告警，不能拿不相关的重启告警顶替。
}

// Truth 保存解释，不参与业务行为或告警规则。
type Truth struct {
	Component   string `yaml:"component"`   // 定位到服务、表或链路。
	Mechanism   string `yaml:"mechanism"`   // 供以后评测分类。
	Explanation string `yaml:"explanation"` // 用自然语言解释原因如何引出症状。
}
