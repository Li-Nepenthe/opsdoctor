// 本文件：在一个事务中去重告警分组、创建任务、保存告警快照。
//
// 不能先查“任务不存在”再直接插入；两个同时到达的请求可能都查到不存在。
// diagnoses 的唯一 active_group 负责最终裁决，事务保证任务和告警一起保存。
//
// 主要内容：
//   - ReceiveAlerts：事务入口，提交成功后才返回接收结果。
//   - pendingGroup：创建或锁住本系统的待处理分组。
//   - saveAlerts：按指纹更新告警快照，不创建重复行。
//
// 阅读建议：先看 active_group 怎样包含系统名，再看并发推送怎样复用同一任务。

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/Li-Nepenthe/opsdoctor/internal/doctor/alerts"
)

// ReceiveAlerts 的事务失败会整体回滚；发送方重试仍受唯一分组约束，不会多建任务。
func (store *Store) ReceiveAlerts(ctx context.Context, target string, value alerts.Notification) (alerts.Receipt, error) {
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return alerts.Receipt{}, err
	}
	// Commit 后 Rollback 返回 ErrTxDone，忽略它；真正失败时这一步负责释放锁。
	defer transaction.Rollback()
	receipt, err := pendingGroup(ctx, transaction, target, value)
	if err != nil {
		return alerts.Receipt{}, err
	}
	if !receipt.Ignored {
		if err := saveAlerts(ctx, transaction, receipt.DiagnosisID, value.Alerts); err != nil {
			return alerts.Receipt{}, err
		}
	}
	if err := transaction.Commit(); err != nil {
		return alerts.Receipt{}, err
	}
	return receipt, nil
}

// pendingGroup 只在 firing 时创建任务，单独收到 resolved 不代表现在需要诊断。
func pendingGroup(ctx context.Context, transaction *sql.Tx, target string, value alerts.Notification) (alerts.Receipt, error) {
	digest := sha256.Sum256([]byte(target + "\x00" + value.GroupKey))
	key := hex.EncodeToString(digest[:])
	receipt := alerts.Receipt{}
	candidate := ""
	if value.Status == "firing" {
		input, err := json.Marshal(value)
		if err != nil {
			return receipt, err
		}
		// 唯一键让同时来的相同分组串行处理；重复时不覆盖最初触发任务的 input。
		candidate = newID()
		_, err = transaction.ExecContext(ctx, "INSERT INTO diagnoses(id,target,`trigger`,status,title,alert_group_key,active_group,input) VALUES(?,?,'alert','pending',?,?,?,?) ON DUPLICATE KEY UPDATE id=id", candidate, target, alerts.Title(value), value.GroupKey, key, input)
		if err != nil {
			return receipt, err
		}

	}
	err := transaction.QueryRowContext(ctx, "SELECT id FROM diagnoses WHERE active_group=? AND target=? FOR UPDATE", key, target).Scan(&receipt.DiagnosisID)
	if errors.Is(err, sql.ErrNoRows) && value.Status == "resolved" {
		receipt.Ignored = true
		return receipt, nil
	}
	// 比较最终 ID，不依赖驱动的 clientFoundRows 选项来判断是否新建。
	receipt.Created = err == nil && candidate != "" && receipt.DiagnosisID == candidate
	return receipt, err
}

// saveAlerts 在同一事务里按指纹保存最新状态；恢复通知不会删除仍待人工查看的任务。
func saveAlerts(ctx context.Context, transaction *sql.Tx, id string, values []alerts.Alert) error {
	for _, value := range values {
		payload, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = transaction.ExecContext(ctx, "INSERT INTO diagnosis_alerts(diagnosis_id,fingerprint,payload) VALUES(?,?,?) ON DUPLICATE KEY UPDATE payload=?,received_at=CURRENT_TIMESTAMP(6)", id, value.Fingerprint, payload, payload)
		if err != nil {
			return err
		}
	}
	return nil
}
