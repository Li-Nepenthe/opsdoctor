// 本文件：补上模型结算和步骤保存之间的崩溃空窗。
//
// 供应商的回复先写进费用审计，再交给 Agent。进程可能恰好在这两步之间退出。
// 恢复时用发送前的请求编号查审计；有成功回复就回放，没有就保留预留费用并记明中断。
//
// 主要内容：recoverModelCalls 找未闭合的请求；recoverOneCall 汇总重试费用并找回回答。
// 阅读建议：恢复不会重新发送同一次请求，也不会把不确定的费用退回预算账户。

package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/agent"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm"
	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/llm/openai"
)

// recoverModelCalls 只查看本次诊断的发送意图，不能用另一个诊断的回答续接对话。
func (store *Store) recoverModelCalls(ctx context.Context, id string) error {
	rows, err := store.database.QueryContext(ctx, "SELECT seq,args FROM diagnosis_steps AS intent WHERE diagnosis_id=? AND kind='system' AND status='llm_pending' AND NOT EXISTS(SELECT 1 FROM diagnosis_steps AS answer WHERE answer.diagnosis_id=intent.diagnosis_id AND answer.event_key=CONCAT('request:',intent.seq)) ORDER BY seq", id)
	if err != nil {
		return err
	}
	type intent struct {
		seq  int
		args json.RawMessage
	}
	var pending []intent
	for rows.Next() {
		var value intent
		if err = rows.Scan(&value.seq, &value.args); err != nil {
			_ = rows.Close()
			return err
		}
		pending = append(pending, value)
	}
	readErr := rows.Err()
	_ = rows.Close()
	if readErr != nil {
		return readErr
	}
	// 先关闭查询结果再写回，避免读连接一直占着有限的连接池。
	for _, value := range pending {
		if err = store.recoverOneCall(ctx, id, fmt.Sprintf("request:%d", value.seq), value.args); err != nil {
			return err
		}
	}
	return nil
}

// recoverOneCall 把 HTTP 重试仍合成一轮模型调用，所有失败尝试的费用都保留。
func (store *Store) recoverOneCall(ctx context.Context, id, key string, request json.RawMessage) error {
	// 已失去执行权的旧网络请求即使以后返回，也不能把这笔未知费用悄悄当成零。
	_, err := store.database.ExecContext(ctx, "UPDATE llm_attempts SET status='interrupted',settled_at=UTC_TIMESTAMP(6) WHERE diagnosis_id=? AND call_id=? AND settled_at IS NULL", id, key)
	if err != nil {
		return err
	}
	rows, err := store.database.QueryContext(ctx, "SELECT id,requested_model,response,charged_micros,status FROM llm_attempts WHERE diagnosis_id=? AND call_id=? ORDER BY created_at,id", id, key)
	if err != nil {
		return err
	}
	var result llm.Response
	status := "interrupted"
	for rows.Next() {
		var attempt, model, state string
		var raw []byte
		var cost int64
		if err = rows.Scan(&attempt, &model, &raw, &cost, &state); err != nil {
			_ = rows.Close()
			return err
		}
		result.AttemptIDs = append(result.AttemptIDs, attempt)
		result.CostMicros += cost
		if result.Model == "" {
			result.Model = model
		}
		if state != "ok" {
			continue
		}
		// 这里只读客户端已经验证并成功结算的回复；损坏的历史不能冒充一次成功查询。
		answer, decodeErr := openai.DecodeSavedResponse(raw, model)
		if decodeErr != nil {
			_ = rows.Close()
			return fmt.Errorf("已结算模型回复无法恢复：%w", decodeErr)
		}
		result.Message, result.Usage, result.Model = answer.Message, answer.Usage, answer.Model
		status = "ok"
	}
	readErr := rows.Err()
	_ = rows.Close()
	if readErr != nil {
		return readErr
	}
	if result.Model == "" {
		var sent llm.Request
		_ = json.Unmarshal(request, &sent)
		result.Model = sent.Model
	}
	raw, _ := json.Marshal(result)
	_, err = store.RecordStep(ctx, id, agent.Step{EventKey: key, CallID: key, Kind: "llm", Model: result.Model, Args: request, Result: string(raw), Status: status, Usage: result.Usage, CostMicros: result.CostMicros})
	return err
}
