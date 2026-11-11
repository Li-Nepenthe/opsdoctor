// 本文件：在开始生产或消费之前创建订单主题，并核对已有主题的分区数。
// EnsureTopic 通过普通 broker 找到控制节点，由控制节点接受创建请求。

package events

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
)

// OrderCreatedTopic 是订单服务与消费者共用的主题名。
const OrderCreatedTopic = "order.created"

// 为主题准备单独限时，给其余启动操作留出时间。
const topicSetupTimeout = 10 * time.Second

// 生产者和消费者共用三个分区的契约。
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
