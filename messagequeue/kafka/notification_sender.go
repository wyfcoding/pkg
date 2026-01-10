package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// NotificationCommand 定义了发送至 Kafka 的统一通知指令协议。
type NotificationCommand struct {
	Target  string `json:"target"`  // 通知目标（如用户 ID、手机号或邮箱）。
	Subject string `json:"subject"` // 通知主题（如“支付成功”、“系统告警”）。
	Content string `json:"content"` // 通知正文。
}

// NotificationSender 封装了通过 Kafka 异步发送通知的逻辑实现。
type NotificationSender struct {
	producer *Producer // 底层 Kafka 生产者。
	topic    string    // 目标消息主题。
}

// NewNotificationSender 创建 Kafka 发送器实例。
func NewNotificationSender(producer *Producer, topic string) *NotificationSender {
	return &NotificationSender{
		producer: producer,
		topic:    topic,
	}
}

// Send 执行通知消息的异步推送。
// 架构设计：利用 Target 作为 Kafka 分区 Key，确保针对同一目标的通知消息保持严格时序。
func (s *NotificationSender) Send(ctx context.Context, target, subject, content string) error {
	cmd := NotificationCommand{
		Target:  target,
		Subject: subject,
		Content: content,
	}

	payload, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal notification command: %w", err)
	}

	if err := s.producer.PublishToTopic(ctx, s.topic, []byte(target), payload); err != nil {
		return err
	}

	slog.InfoContext(ctx, "notification pushed to mq",
		"target", target,
		"subject", subject,
		"topic", s.topic,
	)

	return nil
}
