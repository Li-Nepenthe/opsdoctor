// 本文件：在 Kafka 消息头中传递调用链，消息正文仍只包含订单业务字段。
//
// HTTP 和 gRPC 插件会自动传递上下文，kafka-go 需要我们手动做这一步。
// carrier 是“存放字符串键值的容器”。实现传播器需要的三个方法，就能复用 W3C 编码规则。
//
// 主要内容：
//   - KafkaHeaders：把 kafka.Header 切片适配成 TextMapCarrier。
//   - Get、Set、Keys：读取、替换和列出头字段。
//
// 阅读建议：Set 会去掉同名旧值，避免转发消息后出现两个互相矛盾的 traceparent。

package telemetry

import (
	"strings"

	"github.com/segmentio/kafka-go"
)

// KafkaHeaders 借用当前消息的切片，只在发送前修改，不能与写入 Kafka 的动作并发。
type KafkaHeaders []kafka.Header

// Get 按 HTTP/W3C 头的习惯忽略大小写，不要求发送端恰好使用小写。
func (headers *KafkaHeaders) Get(key string) string {
	for _, header := range *headers {
		if strings.EqualFold(header.Key, key) {
			return string(header.Value)
		}
	}
	return ""
}

// Set 分配自己的切片，既替换重复键，也不修改别人还持有的旧消息头。
func (headers *KafkaHeaders) Set(key, value string) {
	result := make(KafkaHeaders, 0, len(*headers)+1)
	for _, header := range *headers {
		if !strings.EqualFold(header.Key, key) {
			result = append(result, header)
		}
	}
	*headers = append(result, kafka.Header{Key: key, Value: []byte(value)})
}

// Keys 只暴露字段名，传播器通过 Get 读取对应值。
func (headers *KafkaHeaders) Keys() []string {
	keys := make([]string, 0, len(*headers))
	for _, header := range *headers {
		keys = append(keys, header.Key)
	}
	return keys
}
