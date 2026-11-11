// 本文件：把已提交的诊断步骤通知该诊断的订阅者。发布与关闭通道共用一把锁，某个浏览器来不及读取时只断开它的订阅。

package events

import (
	"encoding/json"
	"sync"
)

// BufferSize 为每个订阅者保留 64 条；更旧的内容由数据库补发，避免内存随断网时间增长。
const BufferSize = 64

// Event 的 ID 对应数据库步骤序号，只在一个诊断内部递增。
type Event struct {
	ID   int
	Data json.RawMessage
}

// Hub 的锁保护 groups 及每个 Subscription 的通道生命周期。
type Hub struct {
	mu     sync.Mutex
	groups map[string]map[*Subscription]bool
}

// Subscription 的 channel 由 Hub 写和关闭；HTTP 处理器只读并在退出时调用 Close。
type Subscription struct {
	hub       *Hub
	diagnosis string
	channel   chan Event
}

// New 为一个医生进程建立独立中心，不使用包级全局订阅表。
func New() *Hub { return &Hub{groups: map[string]map[*Subscription]bool{}} }

// Subscribe 必须先于数据库补发执行，避免两者之间恰好新增的步骤落在空窗里。
func (hub *Hub) Subscribe(diagnosis string) *Subscription {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	subscription := &Subscription{hub: hub, diagnosis: diagnosis, channel: make(chan Event, BufferSize)}
	if hub.groups[diagnosis] == nil {
		hub.groups[diagnosis] = map[*Subscription]bool{}
	}
	hub.groups[diagnosis][subscription] = true
	return subscription
}

// Events 只暴露接收方向，调用者不能向别人的订阅塞入伪造步骤。
func (subscription *Subscription) Events() <-chan Event { return subscription.channel }

// Close 可以重复调用，正常退出和满通道断开可能先后发生。
func (subscription *Subscription) Close() {
	hub := subscription.hub
	hub.mu.Lock()
	defer hub.mu.Unlock()
	hub.remove(subscription)
}

// remove 由持锁调用方使用，删除最后一个订阅时也清理诊断分组。
func (hub *Hub) remove(subscription *Subscription) {
	group := hub.groups[subscription.diagnosis]
	if !group[subscription] {
		return
	}
	delete(group, subscription)
	close(subscription.channel)
	if len(group) == 0 {
		delete(hub.groups, subscription.diagnosis)
	}
}

// Publish 不等待慢浏览器；满了就关闭，浏览器随后用数据库序号续传。
func (hub *Hub) Publish(diagnosis string, event Event) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	for subscription := range hub.groups[diagnosis] {
		// 每个订阅得到独立字节，接收者修改自己的内容不会影响另一个浏览器。
		copyEvent := Event{ID: event.ID, Data: append(json.RawMessage{}, event.Data...)}
		select {
		case subscription.channel <- copyEvent:
		default:
			hub.remove(subscription)
		}
	}
}
