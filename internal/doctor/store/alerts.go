// 本文件：把同一次故障的反复通知归到同一个诊断，并判断故障何时结束。
//
// 报告提交并不代表故障恢复。分组行独立保存开放状态，只有整组 resolved 或长时间
// 没有通知才结束这一轮。整组恢复后还会观察一段时间，等较慢的告警到齐。
// 锁住分组行后再建任务，并发通知不会同时创建两份。
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
	"fmt"
	"time"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/alerts"
)

// ReceiveAlerts 先清理敏感文字，再在一个事务里更新分组与快照。
func (store *Store) ReceiveAlerts(ctx context.Context, target string, value alerts.Notification) (alerts.Receipt, error) {
	if store.config.RecoveryWindows[target] <= 0 {
		return alerts.Receipt{}, fmt.Errorf("系统未配置恢复观察期")
	}
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
	// 与评测模式切换按同一顺序加锁，不能把切换之前的快照误用于之后到达的通知。
	var guard int
	if err = tx.QueryRowContext(ctx, "SELECT id FROM scheduler_guard WHERE id=1 FOR UPDATE").Scan(&guard); err != nil {
		return alerts.Receipt{}, err
	}
	// 原分组键只参与哈希，不落明文库；脱敏不能改变推送方的去重身份。
	receipt, err := store.receiveGroup(ctx, tx, target, key, value, clean)
	if err != nil {
		return alerts.Receipt{}, err
	}
	if !receipt.Ignored {
		var recoveryAt *time.Time
		if receipt.RecoveryFollowup {
			now := store.config.Now().UTC()
			recoveryAt = &now
		}
		if err = saveAlerts(ctx, tx, receipt.DiagnosisID, value.Alerts, recoveryAt); err != nil {
			return alerts.Receipt{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return alerts.Receipt{}, err
	}
	if receipt.Created && store.config.Changed != nil {
		state := "pending"
		if receipt.Suppressed {
			state = "cancelled"
		}
		store.config.Changed(target, state, 0)
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
	var opened, closed, recovery sql.NullTime
	var seen time.Time
	if err := tx.QueryRowContext(ctx, "SELECT diagnosis_id,opened_at,closed_at,last_seen_at,recovery_until FROM alert_groups WHERE group_hash=? FOR UPDATE", key).Scan(&id, &opened, &closed, &seen, &recovery); err != nil {
		return alerts.Receipt{}, err
	}
	observing := id.Valid && closed.Valid && recovery.Valid && now.Before(recovery.Time)
	active := id.Valid && !closed.Valid && now.Sub(seen) < store.config.FaultSilence
	if value.Status == "resolved" {
		// 旧一轮的恢复通知迟到时，不能关闭已经重新开始的故障。
		if (!active && !observing) || resolvedBefore(value, opened.Time) {
			return alerts.Receipt{Ignored: true}, nil
		}
		// 同一份 resolved 重试不能不断延长观察期，否则故障永远结束不了。
		until := now.Add(store.config.RecoveryWindows[target])
		_, err := tx.ExecContext(ctx, "UPDATE alert_groups SET recovery_until=COALESCE(recovery_until,?),closed_at=COALESCE(closed_at,?),last_seen_at=? WHERE group_hash=?", until, now, now, key)
		return alerts.Receipt{DiagnosisID: id.String}, err
	}
	if active || observing {
		// 观察期内又响起告警，先恢复为开放状态；下一次整组 resolved 再重新计时。
		// 不改变 diagnoses 的终态，调度器不会为补充告警再跑一轮模型。
		_, err := tx.ExecContext(ctx, "UPDATE alert_groups SET last_seen_at=?,closed_at=NULL,recovery_until=NULL WHERE group_hash=?", now, key)
		return alerts.Receipt{DiagnosisID: id.String, RecoveryFollowup: observing}, err
	}
	// 静默超时后的 firing 是新一轮，旧任务和告警快照仍留在审计表里。
	candidate := newID()
	// 评测期间保留通知和故障开放期，但这条记录直接终止，永远不交给模型收费执行。
	status, reason := "pending", ""
	var evaluating bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM evaluation_targets WHERE target=? AND enabled=TRUE)", target).Scan(&evaluating); err != nil {
		return alerts.Receipt{}, err
	}
	if evaluating {
		status, reason = "cancelled", "评测期间仅记录告警，自动诊断已暂停"
	}
	start := now
	for _, alert := range value.Alerts {
		if !alert.StartsAt.IsZero() && alert.StartsAt.Before(start) {
			start = alert.StartsAt
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO diagnoses(id,target,`trigger`,status,title,alert_group_key,input,fault_started_at,error,finished_at) VALUES(?,?,'alert',?,?,?,?,?,?,IF(?='cancelled',UTC_TIMESTAMP(6),NULL))", candidate, target, status, alerts.Title(value), value.GroupKey, input, start, reason, status); err != nil {
		return alerts.Receipt{}, err
	}
	_, err := tx.ExecContext(ctx, "UPDATE alert_groups SET diagnosis_id=?,opened_at=?,last_seen_at=?,closed_at=NULL,recovery_until=NULL WHERE group_hash=?", candidate, now, now, key)
	return alerts.Receipt{DiagnosisID: candidate, Created: true, Suppressed: evaluating}, err
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
func saveAlerts(ctx context.Context, tx *sql.Tx, id string, values []alerts.Alert, recoveryAt *time.Time) error {
	for _, value := range values {
		payload, err := json.Marshal(value)
		if err != nil {
			return err
		}
		// 同组通知也可能带已恢复的旧告警，只把这次仍 firing 的症状标为后续告警。
		var arrived *time.Time
		if value.Status == "firing" {
			arrived = recoveryAt
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO diagnosis_alerts(diagnosis_id,fingerprint,payload,recovery_received_at) VALUES(?,?,?,?) ON DUPLICATE KEY UPDATE payload=?,received_at=CURRENT_TIMESTAMP(6),recovery_received_at=COALESCE(recovery_received_at,?)", id, value.Fingerprint, payload, arrived, payload, arrived); err != nil {
			return err
		}
	}
	return nil
}
