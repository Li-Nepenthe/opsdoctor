// 本文件：给页面和验收脚本提供故障开放状态，以及恢复观察期补入的告警。
//
// 报告可以先结束，告警稍后才到。这里单独查询这些小记录，不反复读取整份模型审计。
// 观察期是否结束直接按数据库时间字段判断，不需要后台计时任务，也不涉及崩溃恢复。
//
// 主要内容：FaultState 判断系统能否开始下一场实验；DiagnosisAlerts 读取告警快照与来历。
// 阅读建议：RecoveryUntil 为 nil 表示尚未整组恢复；Open 为 false 才表示当前没有开放故障。

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// FaultState 是故障状态，不是诊断状态；终态报告仍可能对应一个开放故障。
type FaultState struct {
	Open          bool       `json:"open"`           // 还有 firing 或仍在恢复观察期内。
	RecoveryUntil *time.Time `json:"recovery_until"` // 全部分组都在观察时给出最晚结束时间，否则为 null。
}

// RecoveryAlert 保留第一次在观察期补入的时间，后续恢复通知只更新告警内容。
type RecoveryAlert struct {
	ReceivedAt time.Time       `json:"received_at"` // 到达医生的时间，便于与报告完成时间对照。
	Alert      json.RawMessage `json:"alert"`       // 和普通告警相同，已按统一规则脱敏。
}

// AlertHistory 可独立刷新，避免报告结束后页面看不到后续到达的症状。
type AlertHistory struct {
	Alerts         []json.RawMessage `json:"alerts"`          // 每条告警的最新状态。
	RecoveryAlerts []RecoveryAlert   `json:"recovery_alerts"` // 曾在观察期补入的告警单独列出。
}

// FaultState 忽略已经静默超时的旧分组；多个分组中只要一个未恢复，结束时间就还未知。
func (store *Store) FaultState(ctx context.Context, target string) (FaultState, error) {
	now := store.config.Now().UTC()
	rows, err := store.database.QueryContext(ctx, "SELECT closed_at,recovery_until FROM alert_groups WHERE target=? AND diagnosis_id IS NOT NULL AND ((closed_at IS NULL AND last_seen_at>?) OR (closed_at IS NOT NULL AND recovery_until>?))", target, now.Add(-store.config.FaultSilence), now)
	if err != nil {
		return FaultState{}, err
	}
	defer rows.Close()
	value := FaultState{}
	active := false
	for rows.Next() {
		var closed, until sql.NullTime
		if err := rows.Scan(&closed, &until); err != nil {
			return value, err
		}
		value.Open = true
		if !closed.Valid {
			active = true
		} else if value.RecoveryUntil == nil || until.Time.After(*value.RecoveryUntil) {
			value.RecoveryUntil = &until.Time
		}
	}
	// 不能把某个已恢复分组的时间误当成整个系统会结束的承诺。
	if active {
		value.RecoveryUntil = nil
	}
	return value, rows.Err()
}

// DiagnosisAlerts 用左连接区分“不存在的诊断”和“没有告警的手动诊断”。
func (store *Store) DiagnosisAlerts(ctx context.Context, id string) (AlertHistory, error) {
	value := AlertHistory{Alerts: []json.RawMessage{}, RecoveryAlerts: []RecoveryAlert{}}
	rows, err := store.database.QueryContext(ctx, "SELECT a.payload,a.recovery_received_at FROM diagnoses d LEFT JOIN diagnosis_alerts a ON a.diagnosis_id=d.id WHERE d.id=? ORDER BY a.fingerprint", id)
	if err != nil {
		return value, err
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		found = true
		var payload []byte
		var arrived sql.NullTime
		if err := rows.Scan(&payload, &arrived); err != nil {
			return value, err
		}
		if payload == nil {
			continue // 手动任务没有关联行，返回空数组而不是一条 null 告警。
		}
		value.Alerts = append(value.Alerts, json.RawMessage(payload))
		if arrived.Valid {
			value.RecoveryAlerts = append(value.RecoveryAlerts, RecoveryAlert{ReceivedAt: arrived.Time, Alert: json.RawMessage(payload)})
		}
	}
	if err := rows.Err(); err != nil {
		return value, err
	}
	if !found {
		return value, sql.ErrNoRows
	}
	return value, nil
}
