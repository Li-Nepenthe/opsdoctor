// 本文件：在接收业务流量之前准备订单消息主题。
//
// 生产者和消费者都依赖同一个三分区主题。显式检查可以尽早发现手工创建错分区的情况。
//
// 主要内容：
//   - OrderCreatedTopic：统一主题名。
//   - EnsureTopic：连接 Kafka 控制节点，创建主题并核对分区数量。
//
// 阅读建议：普通 broker 能回答控制节点地址，但创建主题的请求要发送给控制节点。

package events

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
)

// OrderCreatedTopic 是跨进程契约，不允许生产者和消费者各写一个名字。
const OrderCreatedTopic = "order.created"

// 主题准备是启动中的一个步骤，不能用光整个进程的三十秒启动预算。
const topicSetupTimeout = 10 * time.Second

// orderPartitions 来自计划书的固定契约，生产与消费启动时都按这个值检查。
const orderPartitions = 3

// EnsureTopic 只创建计划书约定的主题，不修改已有主题的分区或删除数据。
func EnsureTopic(parent context.Context, brokers []string) error {
	if len(brokers) == 0 {
		return fmt.Errorf("缺少 Kafka broker 地址")
	}
	ctx, cancel := context.WithTimeout(parent, topicSetupTimeout)
	defer cancel()
	connection, err := kafka.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		return fmt.Errorf("连接 Kafka：%w", err)
	}
	defer connection.Close()
	deadline, _ := ctx.Deadline()
	if err := connection.SetDeadline(deadline); err != nil {
		return err
	}
	controller, err := connection.Controller()
	if err != nil {
		return fmt.Errorf("查询 Kafka 控制节点：%w", err)
	}
	control, err := kafka.DialContext(ctx, "tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return fmt.Errorf("连接 Kafka 控制节点：%w", err)
	}
	defer control.Close()
	if err := control.SetDeadline(deadline); err != nil {
		return err
	}
	// 本机只有一个 broker，因此副本数为 1。这不提供生产环境所需的节点冗余。
	if err := control.CreateTopics(kafka.TopicConfig{Topic: OrderCreatedTopic, NumPartitions: orderPartitions, ReplicationFactor: 1}); err != nil {
		return fmt.Errorf("创建订单主题：%w", err)
	}
	partitions, err := connection.ReadPartitions(OrderCreatedTopic)
	if err != nil {
		return fmt.Errorf("检查订单主题：%w", err)
	}
	if len(partitions) != orderPartitions {
		return fmt.Errorf("订单主题需要 3 个分区，实际为 %d", len(partitions))
	}
	return nil
}
