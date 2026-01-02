package kafka

import (
	"context"
	"encoding/json"
	"fmt"
)

// NotificationCommand 发送到 Kafka 的统一指令格式
type NotificationCommand struct {
	Target  string `json:"target"`
	Subject string `json:"subject"`
	Content string `json:"content"`
}

// KafkaNotificationSender 将通知指令发送到 Kafka
type KafkaNotificationSender struct {
	producer *Producer
	topic    string
}

// NewKafkaNotificationSender 创建 Kafka 发送器实例
func NewKafkaNotificationSender(producer *Producer, topic string) *KafkaNotificationSender {
	return &KafkaNotificationSender{
		producer: producer,
		topic:    topic,
	}
}

// Send 将通知推送到消息队列
func (s *KafkaNotificationSender) Send(ctx context.Context, target, subject, content string) error {
	cmd := NotificationCommand{
		Target:  target,
		Subject: subject,
		Content: content,
	}

	payload, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal notification command: %w", err)
	}

	// 使用 Target 做 Key 保证同一接收者的消息顺序
	return s.producer.PublishToTopic(ctx, s.topic, []byte(target), payload)
}
