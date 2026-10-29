// 本文件：把三个固定写工具的参数转换成可审阅、可撤销的计划。
//
// 模型不能传入 SQL 或索引名。表和列必须来自架构说明与真实元数据，程序才会
// 拼出 CREATE INDEX。其他两个工具只能选择配置里列出的服务和有限商品编号。
//
// 主要内容：decodeProposal 检查格式；prepare 检查证据和范围；indexPlan 生成确切语句。
// 阅读建议：这里的数据库访问都只读，真正执行留到审批通过以后。

package approval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
)

// proposal 只用于读取模型输入；不接受 URL、主机、系统名或任意 SQL。
type proposal struct {
	Table    string   `json:"table"`
	Columns  []string `json:"columns"`
	Unique   bool     `json:"unique"`
	Service  string   `json:"service"`
	Size     int      `json:"size"`
	IDs      []int64  `json:"ids"`
	Reason   string   `json:"reason"`
	Evidence []int    `json:"evidence_steps"`
	Expected string   `json:"expected_effect"`
	Risk     string   `json:"risk"`
}

// decodeProposal 逐个读键，拒绝“展示一个值、执行时被第二个同名键覆盖”的输入。
func decodeProposal(raw json.RawMessage, tool string) (proposal, error) {
	var input proposal
	if len(raw) == 0 || len(raw) > 8192 {
		return input, fmt.Errorf("写工具参数必须是最多 8192 字节的对象")
	}
	// 每种申请只有自己的字段。不能通过多带一个字段把当前系统换掉。
	allowed := map[string]bool{"reason": true, "evidence_steps": true, "expected_effect": true, "risk": true}
	switch tool {
	case "create_index":
		allowed["table"], allowed["columns"], allowed["unique"] = true, true, true
	case "set_db_pool_size":
		allowed["service"], allowed["size"] = true, true
	case "warm_cache":
		allowed["ids"] = true
	default:
		return input, fmt.Errorf("未定义的写工具")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return input, fmt.Errorf("写工具参数必须是对象")
	}
	seen := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return input, fmt.Errorf("参数 JSON 格式错误")
		}
		name, ok := key.(string)
		if !ok || seen[name] || !allowed[name] {
			return input, fmt.Errorf("参数重复或含未定义字段，不接受主机、URL、系统名或 SQL")
		}
		seen[name] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return input, fmt.Errorf("参数 JSON 格式错误")
		}
	}
	if _, err = decoder.Token(); err != nil {
		return input, fmt.Errorf("参数 JSON 未结束")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || json.Unmarshal(raw, &input) != nil {
		return input, fmt.Errorf("参数类型错误或包含多个对象")
	}
	// JSON Schema 只是告诉模型怎么填；执行端仍要检查必填项，false 也必须明确传入。
	for name := range allowed {
		if !seen[name] {
			return input, fmt.Errorf("缺少必填参数 %s", name)
		}
	}
	return input, nil
}

// prepare 限制申请文字和证据数量。错误交回模型修正，不会创建一张没有依据的审批。
func (backend *Backend) prepare(ctx context.Context, id, tool string, raw json.RawMessage, steps map[int]bool) (Record, error) {
	input, err := decodeProposal(raw, tool)
	if err != nil {
		return Record{}, err
	}
	for _, text := range []string{input.Reason, input.Expected, input.Risk} {
		if strings.TrimSpace(text) == "" || len(text) > 2048 {
			return Record{}, fmt.Errorf("理由、预期效果和风险都必须填写，且各不超过 2048 字节")
		}
	}
	if len(input.Evidence) < 1 || len(input.Evidence) > 10 {
		return Record{}, fmt.Errorf("证据需要 1–10 个本次诊断的步骤号")
	}
	// 证据必须来自这次诊断，不能拿另一份报告的步骤号当授权依据。
	seen := map[int]bool{}
	for _, step := range input.Evidence {
		if !steps[step] || seen[step] {
			return Record{}, fmt.Errorf("证据步骤不存在或重复")
		}
		seen[step] = true
	}
	plan := Plan{Tool: tool}
	switch tool {
	case "create_index":
		plan, err = backend.indexPlan(ctx, id, input)
		if err != nil {
			return Record{}, err
		}
	case "set_db_pool_size":
		if backend.ops.Services[input.Service] == "" || input.Size < backend.ops.PoolMin || input.Size > backend.ops.PoolMax {
			return Record{}, fmt.Errorf("服务未配置，或连接池值不在 %d–%d 内", backend.ops.PoolMin, backend.ops.PoolMax)
		}
		var before PoolState
		err = backend.opsRequest(ctx, input.Service, "GET", "/ops/pool", nil, &before)
		if err == nil {
			plan = Plan{Tool: tool, Service: input.Service, Size: input.Size, Before: before, Preview: fmt.Sprintf("把 %s 当前进程的连接池上限从 %d 改为 %d；实例 %s，修订 %d", input.Service, before.Current, input.Size, before.Instance, before.Revision), Rollback: fmt.Sprintf("重启 %s，连接池恢复启动配置 %d；或由人另行提交审批恢复旧值 %d。", input.Service, before.Initial, before.Current)}
		}
	case "warm_cache":
		if backend.ops.WarmService == "" || len(input.IDs) < 1 || len(input.IDs) > 100 {
			return Record{}, fmt.Errorf("未配置商品预热服务，或商品 ID 数量不在 1–100 内")
		}
		ids := map[int64]bool{}
		for _, id := range input.IDs {
			if id < 1 || ids[id] {
				return Record{}, fmt.Errorf("商品 ID 必须为正整数且不重复")
			}
			ids[id] = true
		}
		encoded, _ := json.Marshal(input.IDs)
		plan = Plan{Tool: tool, Service: backend.ops.WarmService, IDs: input.IDs, Preview: "只补充以下商品缺失的缓存：" + string(encoded), Rollback: "不修改业务数据，无需撤销。缓存按原 TTL 自然过期。"}
	}
	if err != nil {
		return Record{}, fmt.Errorf("准备审批时读取真实状态失败，请检查数据源后重试")
	}
	return Record{Tool: tool, Args: raw, Plan: plan, Reason: input.Reason, Evidence: input.Evidence, Expected: input.Expected, Risk: input.Risk}, nil
}

// indexPlan 先核对表、列，再引用标识符；只允许常规名字，不让字符串变成 SQL 片段。
func (backend *Backend) indexPlan(ctx context.Context, id string, input proposal) (Plan, error) {
	if backend.ddl == nil || backend.metadata == nil || !backend.tables[input.Table] {
		return Plan{}, fmt.Errorf("未配置索引能力或表不在系统说明书中")
	}
	if len(input.Columns) < 1 || len(input.Columns) > 3 {
		return Plan{}, fmt.Errorf("索引必须包含 1–3 列")
	}
	columns, err := backend.metadata.Columns(ctx, input.Table)
	if err != nil {
		return Plan{}, fmt.Errorf("无法读取允许表的列，请检查元数据来源")
	}
	// 值存在于元数据还不够：名字也不能带 SQL 符号，同一列不能重复。
	seen := map[string]bool{}
	quoted := []string{}
	for _, column := range input.Columns {
		if !identifier(column) || !slices.Contains(columns, column) || seen[column] {
			return Plan{}, fmt.Errorf("索引列不存在、重复或名字不合法")
		}
		seen[column] = true
		quoted = append(quoted, "`"+column+"`")
	}
	name := "idx_doctor_" + id[:20]
	unique := ""
	if input.Unique {
		unique = "UNIQUE "
	}
	sql := fmt.Sprintf("CREATE %sINDEX `%s` ON `%s` (%s) ALGORITHM=INPLACE LOCK=NONE", unique, name, input.Table, strings.Join(quoted, ","))
	return Plan{Tool: "create_index", Table: input.Table, Columns: input.Columns, Unique: input.Unique, Index: name, Preview: sql, Rollback: fmt.Sprintf("DROP INDEX `%s` ON `%s` ALGORITHM=INPLACE LOCK=NONE;", name, input.Table)}, nil
}

// identifier 把允许的 SQL 标识符范围收窄，不接受点号、引号或转义。
func identifier(value string) bool {
	return regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`).MatchString(value)
}
