// 本文件：通过 Kafka 消息头在生产者和消费者之间传递调用链，消息正文仍只包含业务数据。

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

// Set 替换所有同名旧值，转发时才不会送出两个冲突的 traceparent。旧消息头可能仍由调用方持有。
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
