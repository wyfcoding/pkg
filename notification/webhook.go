package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// WebhookConfig 定义 Webhook 发送器配置。
type WebhookConfig struct {
	// Timeout 请求超时。
	Timeout time.Duration `json:"timeout"`
	// UserAgent 自定义 User-Agent。
	UserAgent string `json:"user_agent"`
}

// WebhookSender 实现 Webhook 发送器。
type WebhookSender struct {
	config *WebhookConfig
	client *http.Client
	logger *slog.Logger
}

// NewWebhookSender 创建 Webhook 发送器实例。
func NewWebhookSender(cfg *WebhookConfig, logger *slog.Logger) *WebhookSender {
	if cfg == nil {
		cfg = &WebhookConfig{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = "Pkg-Notification-Service/1.0"
	}

	return &WebhookSender{
		config: &WebhookConfig{
			Timeout:   timeout,
			UserAgent: userAgent,
		},
		client: &http.Client{Timeout: timeout},
		logger: logger,
	}
}

// Channel 返回渠道类型。
func (s *WebhookSender) Channel() Channel {
	return ChannelWebhook
}

// Send 发送单条 Webhook。
// msg.Recipient 必须是 Webhook URL。
// msg.Content 是 Payload (JSON 字符串)。
func (s *WebhookSender) Send(ctx context.Context, msg *Message) (*Result, error) {
	if msg == nil {
		return nil, fmt.Errorf("webhook message is nil")
	}
	start := time.Now()
	result := &Result{
		MessageID: msg.ID,
		Channel:   ChannelWebhook,
		SentAt:    start,
	}

	target := msg.Recipient
	if target == "" {
		return nil, fmt.Errorf("webhook url (recipient) is empty")
	}

	content := msg.Content
	// 尝试验证 content 是否为有效 JSON，若不是则封装
	if !json.Valid([]byte(content)) {
		payload := map[string]string{
			"subject": msg.Subject,
			"content": content,
		}
		b, _ := json.Marshal(payload)
		content = string(b)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", target, bytes.NewBufferString(content))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.config.UserAgent != "" {
		req.Header.Set("User-Agent", s.config.UserAgent)
	}

	// 添加 Metadata 到 Header
	for k, v := range msg.Metadata {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		result.Status = StatusFailed
		result.Error = err.Error()
		s.logger.ErrorContext(ctx, "webhook send failed",
			"url", target,
			"duration", time.Since(start),
			"error", err)
		return result, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("webhook request failed with status: %s, body: %s", resp.Status, string(body))
		result.Status = StatusFailed
		result.Error = err.Error()
		s.logger.ErrorContext(ctx, "webhook returned error status",
			"url", target,
			"status", resp.StatusCode,
			"error", err)
		return result, err
	}

	result.Status = StatusSent
	s.logger.InfoContext(ctx, "webhook sent successfully",
		"url", target,
		"duration", time.Since(start))

	return result, nil
}

// SendBatch 批量发送 Webhook（串行发送）。
func (s *WebhookSender) SendBatch(ctx context.Context, msgs []*Message) ([]*Result, error) {
	results := make([]*Result, 0, len(msgs))
	for _, msg := range msgs {
		result, err := s.Send(ctx, msg)
		if err != nil {
			s.logger.ErrorContext(ctx, "batch webhook send failed for message",
				"message_id", msg.ID,
				"error", err)
		}
		results = append(results, result)
	}
	return results, nil
}

// Close 关闭发送器。
func (s *WebhookSender) Close() error {
	s.client.CloseIdleConnections()
	return nil
}
