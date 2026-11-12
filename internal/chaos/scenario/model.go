// 本文件：宿主机的场景文件同时描述操作、恢复与验收条件。
// Scenario 保存完整记录，Step 使用固定字段，加载器据此拒绝未知输入。

package scenario

// Scenario 把实验操作与验收条件放在一起，避免两者各自修改后不再对应。
type Scenario struct {
	Rules       []EvidenceRule `yaml:"evidence_rules,omitempty"` // 只有可机械核对的证据才带规则。
	ID          string         `yaml:"id"`                       // 文件名须与已登记的编号一致。
	Title       string         `yaml:"title"`                    // 只向操作者显示。
	Inject      []Step         `yaml:"inject"`                   // 按顺序执行，任一步失败都需要恢复。
	Restore     []Step         `yaml:"restore"`                  // 显式列出恢复顺序，数据库恢复尤其不能猜。
	Symptom     Symptom        `yaml:"symptom"`                  // 用实际指标判断症状，不读取注入器自己的状态。
	GroundTruth Truth          `yaml:"ground_truth"`             // 仅供事后评测使用。
	Evidence    []string       `yaml:"evidence_milestones"`
}

// Step 只允许已经实现的动作。加载器会拒绝未知字段，防止拼错名字后静默失效。
type Step struct {
	Action      string            `yaml:"action"`
	Service     string            `yaml:"service,omitempty"`     // 允许操作的 Compose 服务。
	Environment map[string]string `yaml:"environment,omitempty"` // 只接受不含秘密的配置白名单。
	Table       string            `yaml:"table,omitempty"`
	Index       string            `yaml:"index,omitempty"`
	Proxy       string            `yaml:"proxy,omitempty"` // 三条预先登记的链路之一。
	LatencyMS   int               `yaml:"latency_ms,omitempty"`
	Stream      string            `yaml:"stream,omitempty"`
	QPS         *int              `yaml:"qps,omitempty"`        // 省略时保持原设置，零表示暂停。
	HotSKUID    *int64            `yaml:"hot_sku_id,omitempty"` // 0 恢复正常选品，正数集中商品请求。
	FirstSKUID  int64             `yaml:"first_sku_id,omitempty"`
	Count       int               `yaml:"count,omitempty"`    // 缓存范围最多一万，突发批次最多五百；按动作校验。
	Requests    int               `yaml:"requests,omitempty"` // 每批最多 64 个查询，还受 loadgen 的工作者数量限制。
	TTLSeconds  int               `yaml:"ttl_seconds,omitempty"`
	Image       string            `yaml:"image,omitempty"` // 只允许两个事先构建的本项目订单镜像。
}

// Symptom 同时规定业务观测条件和最长等待时间。
type Symptom struct {
	Healthy bool     `yaml:"healthy,omitempty"` // 健康对照不等待故障告警。
	PromQL  string   `yaml:"promql"`            // 使用不带 bool 的比较表达式；返回有限样本就表示条件成立。
	Within  string   `yaml:"within"`            // 必须为不超过三分钟的正时长。
	Alerts  []string `yaml:"alerts"`            // 至少响过其中一条症状告警，不能拿不相关的重启告警顶替。
}

// Truth 保存解释，不参与业务行为或告警规则。
type Truth struct {
	Component   string `yaml:"component"`
	Mechanism   string `yaml:"mechanism"`
	Explanation string `yaml:"explanation"`
}

// EvidenceRule 只用于事后评分，编号对应人类可读的里程碑，不参与注入或医生工具。
type EvidenceRule struct {
	Milestone int    `yaml:"milestone"` // 从一开始，不能越过 evidence_milestones。
	Kind      string `yaml:"kind"`
	Table     string `yaml:"table"`
	Index     string `yaml:"index,omitempty"` // 判断缺失时要求精确索引名。
}
