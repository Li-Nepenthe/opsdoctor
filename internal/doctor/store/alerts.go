// 本文件：把同一次故障的反复通知归到同一个诊断，并判断故障何时结束。
//
// 报告提交并不代表故障恢复。分组行独立保存开放状态，只有整组 resolved 或长时间
// 没有通知才结束这一轮。锁住分组行后再建任务，并发通知不会同时创建两份。
//
// 主要内容：ReceiveAlerts 保存通知事务；receiveGroup 判断新旧故障；saveAlerts 更新快照。
// 阅读建议：诊断的终态不参与 receiveGroup 的判定，所以结束的诊断仍能接收补充告警。

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/alerts"
)

// ReceiveAlerts 先清理敏感文字，再在一个事务里更新分组与快照。
func (store *Store) ReceiveAlerts(ctx context.Context, target string, value alerts.Notification) (alerts.Receipt, error) {
	key := value.GroupKey
	raw, err := json.Marshal(value)
	if err != nil {
		return alerts.Receipt{}, err
	}
	clean, err := store.config.Redactor.JSON(raw)
	if err != nil {
		return alerts.Receipt{}, err
	}
	// 结构体按值传入仍共享里面的 map 和 slice，必须解析到新对象，不能改调用者的数据。
	var cleaned alerts.Notification
	if err = json.Unmarshal(clean, &cleaned); err != nil {
		return alerts.Receipt{}, err
	}
	value = cleaned
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return alerts.Receipt{}, err
	}
	defer tx.Rollback()
	// 原分组键只参与哈希，不落明文库；脱敏不能改变推送方的去重身份。
	receipt, err := store.receiveGroup(ctx, tx, target, key, value, clean)
	if err != nil {
		return alerts.Receipt{}, err
	}
	if !receipt.Ignored {
		if err = saveAlerts(ctx, tx, receipt.DiagnosisID, value.Alerts); err != nil {
			return alerts.Receipt{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return alerts.Receipt{}, err
	}
	if receipt.Created && store.config.Changed != nil {
		store.config.Changed(target, "pending", 0)
	}
	return receipt, nil
}

// receiveGroup 的 INSERT 先保证有可锁的一行，resolved 先到也不需要创建空诊断。
func (store *Store) receiveGroup(ctx context.Context, tx *sql.Tx, target, originalKey string, value alerts.Notification, input []byte) (alerts.Receipt, error) {
	digest := sha256.Sum256([]byte(target + "\x00" + originalKey))
	key := hex.EncodeToString(digest[:])
	now := store.config.Now().UTC()
	if _, err := tx.ExecContext(ctx, "INSERT INTO alert_groups(group_hash,target,last_seen_at) VALUES(?,?,?) ON DUPLICATE KEY UPDATE group_hash=group_hash", key, target, now); err != nil {
		return alerts.Receipt{}, err
	}
	var id sql.NullString
	var opened, closed sql.NullTime
	var seen time.Time
	if err := tx.QueryRowContext(ctx, "SELECT diagnosis_id,opened_at,closed_at,last_seen_at FROM alert_groups WHERE group_hash=? FOR UPDATE", key).Scan(&id, &opened, &closed, &seen); err != nil {
		return alerts.Receipt{}, err
	}
	active := id.Valid && !closed.Valid && now.Sub(seen) < store.config.FaultSilence
	if value.Status == "resolved" {
		// 旧一轮的恢复通知迟到时，不能关闭已经重新开始的故障。
		if !active || resolvedBefore(value, opened.Time) {
			return alerts.Receipt{Ignored: true}, nil
		}
		_, err := tx.ExecContext(ctx, "UPDATE alert_groups SET closed_at=?,last_seen_at=? WHERE group_hash=?", now, now, key)
		return alerts.Receipt{DiagnosisID: id.String}, err
	}
	if active {
		_, err := tx.ExecContext(ctx, "UPDATE alert_groups SET last_seen_at=? WHERE group_hash=?", now, key)
		return alerts.Receipt{DiagnosisID: id.String}, err
	}
	// 静默超时后的 firing 是新一轮，旧任务和告警快照仍留在审计表里。
	candidate := newID()
	if _, err := tx.ExecContext(ctx, "INSERT INTO diagnoses(id,target,`trigger`,status,title,alert_group_key,input) VALUES(?,?,'alert','pending',?,?,?)", candidate, target, alerts.Title(value), value.GroupKey, input); err != nil {
		return alerts.Receipt{}, err
	}
	_, err := tx.ExecContext(ctx, "UPDATE alert_groups SET diagnosis_id=?,opened_at=?,last_seen_at=?,closed_at=NULL WHERE group_hash=?", candidate, now, now, key)
	return alerts.Receipt{DiagnosisID: candidate, Created: true}, err
}

// resolvedBefore 只在通知确实带结束时间时挡住迟到恢复；旧客户端缺时间时保持兼容。
func resolvedBefore(value alerts.Notification, opened time.Time) bool {
	for _, alert := range value.Alerts {
		if alert.EndsAt.IsZero() || !alert.EndsAt.Before(opened) {
			return false
		}
	}
	return true
}

// saveAlerts 保留每条告警的最近快照；恢复通知不删除诊断，也不触发第二次模型调用。
func saveAlerts(ctx context.Context, tx *sql.Tx, id string, values []alerts.Alert) error {
	for _, value := range values {
		payload, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO diagnosis_alerts(diagnosis_id,fingerprint,payload) VALUES(?,?,?) ON DUPLICATE KEY UPDATE payload=?,received_at=CURRENT_TIMESTAMP(6)", id, value.Fingerprint, payload, payload); err != nil {
			return err
		}
	}
	return nil
}
