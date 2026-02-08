// Package notification 提供 Push 推送发送实现。
// 生成摘要：
// 1) 支持 APNs (iOS) 和 FCM (Android/Web) 推送
// 2) 支持静默推送和富媒体推送
// 假设：
// - APNs 需要 P8 证书或 P12 证书
// - FCM 需要服务账号密钥
package notification

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

// PushProvider 表示推送服务商类型。
type PushProvider string

const (
	// PushProviderAPNs Apple Push Notification service.
	PushProviderAPNs PushProvider = "apns"
	// PushProviderFCM Firebase Cloud Messaging.
	PushProviderFCM PushProvider = "fcm"
)

// PushConfig 定义 Push 发送器配置。
type PushConfig struct {
	// Provider 推送服务商。
	Provider PushProvider `json:"provider"`
	// APNs 配置
	// APNsKeyID P8 证书 Key ID。
	APNsKeyID string `json:"apns_key_id"`
	// APNsTeamID Apple Team ID。
	APNsTeamID string `json:"apns_team_id"`
	// APNsKeyPath P8 证书路径。
	APNsKeyPath string `json:"apns_key_path"`
	// APNsBundleID App Bundle ID。
	APNsBundleID string `json:"apns_bundle_id"`
	// APNsProduction 是否生产环境。
	APNsProduction bool `json:"apns_production"`
	// FCM 配置
	// FCMProjectID Firebase 项目 ID。
	FCMProjectID string `json:"fcm_project_id"`
	// FCMServiceAccountKey 服务账号密钥 JSON。
	FCMServiceAccountKey string `json:"fcm_service_account_key"`
	// Timeout 请求超时。
	Timeout time.Duration `json:"timeout"`
}

// PushPayload 推送载荷。
type PushPayload struct {
	// Title 推送标题。
	Title string `json:"title"`
	// Body 推送正文。
	Body string `json:"body"`
	// Badge 角标数（iOS）。
	Badge *int `json:"badge,omitempty"`
	// Sound 提示音。
	Sound string `json:"sound,omitempty"`
	// ImageURL 图片URL（富媒体推送）。
	ImageURL string `json:"image_url,omitempty"`
	// Data 自定义数据。
	Data map[string]string `json:"data,omitempty"`
	// Silent 静默推送。
	Silent bool `json:"silent,omitempty"`
	// Category 通知类别（iOS）。
	Category string `json:"category,omitempty"`
	// ClickAction 点击动作（Android）。
	ClickAction string `json:"click_action,omitempty"`
	// ChannelID 通知渠道ID（Android 8.0+）。
	ChannelID string `json:"channel_id,omitempty"`
	// Priority 优先级。
	Priority string `json:"priority,omitempty"`
	// TTL 消息有效期（秒）。
	TTL int `json:"ttl,omitempty"`
	// CollapseKey 折叠键。
	CollapseKey string `json:"collapse_key,omitempty"`
}

// PushSender 实现 Push 推送发送器。
type PushSender struct {
	config     *PushConfig
	httpClient *http.Client
	logger     *slog.Logger
	// APNs JWT token 缓存
	apnsToken       string
	apnsTokenExpiry time.Time
}

// NewPushSender 创建 Push 发送器实例。
func NewPushSender(cfg *PushConfig, logger *slog.Logger) (*PushSender, error) {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// 创建支持 HTTP/2 的客户端（APNs 要求）
	transport := &http2.Transport{
		TLSClientConfig: &tls.Config{},
	}

	sender := &PushSender{
		config: cfg,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		logger: logger,
	}

	return sender, nil
}

// Channel 返回渠道类型。
func (s *PushSender) Channel() Channel {
	return ChannelPush
}

// Send 发送单条推送。
func (s *PushSender) Send(ctx context.Context, msg *Message) (*Result, error) {
	start := time.Now()
	result := &Result{
		MessageID: msg.ID,
		Channel:   ChannelPush,
		SentAt:    start,
	}

	// 解析推送载荷
	payload := s.parsePayload(msg)

	var err error
	switch s.config.Provider {
	case PushProviderAPNs:
		result.ThirdPartyID, err = s.sendAPNs(ctx, msg.Recipient, payload)
	case PushProviderFCM:
		result.ThirdPartyID, err = s.sendFCM(ctx, msg.Recipient, payload)
	default:
		err = fmt.Errorf("unsupported push provider: %s", s.config.Provider)
	}

	if err != nil {
		result.Status = StatusFailed
		result.Error = err.Error()
		s.logger.ErrorContext(ctx, "push send failed",
			"provider", s.config.Provider,
			"device_token", s.maskToken(msg.Recipient),
			"duration", time.Since(start),
			"error", err)
		return result, err
	}

	result.Status = StatusSent
	s.logger.InfoContext(ctx, "push sent successfully",
		"provider", s.config.Provider,
		"device_token", s.maskToken(msg.Recipient),
		"apns_id", result.ThirdPartyID,
		"duration", time.Since(start))

	return result, nil
}

// SendBatch 批量发送推送。
func (s *PushSender) SendBatch(ctx context.Context, msgs []*Message) ([]*Result, error) {
	results := make([]*Result, 0, len(msgs))
	for _, msg := range msgs {
		result, err := s.Send(ctx, msg)
		if err != nil {
			s.logger.ErrorContext(ctx, "batch push send failed for message",
				"message_id", msg.ID,
				"error", err)
		}
		results = append(results, result)
	}
	return results, nil
}

// Close 关闭发送器。
func (s *PushSender) Close() error {
	s.httpClient.CloseIdleConnections()
	return nil
}

// parsePayload 解析消息为推送载荷。
func (s *PushSender) parsePayload(msg *Message) *PushPayload {
	payload := &PushPayload{
		Title: msg.Subject,
		Body:  msg.Content,
		Data:  msg.Variables,
	}

	// 从 metadata 解析额外配置
	if msg.Metadata != nil {
		if sound, ok := msg.Metadata["sound"]; ok {
			payload.Sound = sound
		}
		if imageURL, ok := msg.Metadata["image_url"]; ok {
			payload.ImageURL = imageURL
		}
		if category, ok := msg.Metadata["category"]; ok {
			payload.Category = category
		}
		if clickAction, ok := msg.Metadata["click_action"]; ok {
			payload.ClickAction = clickAction
		}
		if channelID, ok := msg.Metadata["channel_id"]; ok {
			payload.ChannelID = channelID
		}
		if msg.Metadata["silent"] == "true" {
			payload.Silent = true
		}
	}

	return payload
}

// sendAPNs 通过 APNs 发送推送。
func (s *PushSender) sendAPNs(ctx context.Context, deviceToken string, payload *PushPayload) (string, error) {
	// 确定 APNs 端点
	var endpoint string
	if s.config.APNsProduction {
		endpoint = "https://api.push.apple.com/3/device/"
	} else {
		endpoint = "https://api.sandbox.push.apple.com/3/device/"
	}
	endpoint += deviceToken

	// 构建 APNs 载荷
	aps := map[string]any{}

	if payload.Silent {
		aps["content-available"] = 1
	} else {
		alert := map[string]string{
			"title": payload.Title,
			"body":  payload.Body,
		}
		aps["alert"] = alert

		if payload.Badge != nil {
			aps["badge"] = *payload.Badge
		}
		if payload.Sound != "" {
			aps["sound"] = payload.Sound
		}
		if payload.Category != "" {
			aps["category"] = payload.Category
		}
	}

	apnsPayload := map[string]any{
		"aps": aps,
	}

	// 添加自定义数据
	for k, v := range payload.Data {
		apnsPayload[k] = v
	}

	bodyBytes, err := json.Marshal(apnsPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// 设置 APNs 请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apns-topic", s.config.APNsBundleID)
	req.Header.Set("apns-push-type", s.getPushType(payload))

	if payload.Priority == "high" {
		req.Header.Set("apns-priority", "10")
	} else {
		req.Header.Set("apns-priority", "5")
	}

	if payload.TTL > 0 {
		expiry := time.Now().Add(time.Duration(payload.TTL) * time.Second)
		req.Header.Set("apns-expiration", fmt.Sprintf("%d", expiry.Unix()))
	}

	if payload.CollapseKey != "" {
		req.Header.Set("apns-collapse-id", payload.CollapseKey)
	}

	// JWT 认证
	token, err := s.getAPNsToken()
	if err != nil {
		return "", fmt.Errorf("failed to get APNs token: %w", err)
	}
	req.Header.Set("Authorization", "bearer "+token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 获取 APNs ID
	apnsID := resp.Header.Get("apns-id")

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var errResp struct {
			Reason    string `json:"reason"`
			Timestamp int64  `json:"timestamp,omitempty"`
		}
		json.Unmarshal(body, &errResp)
		return apnsID, fmt.Errorf("APNs error: status=%d, reason=%s", resp.StatusCode, errResp.Reason)
	}

	return apnsID, nil
}

// getAPNsToken 获取或刷新 APNs JWT token。
func (s *PushSender) getAPNsToken() (string, error) {
	// 简化实现，实际需要使用 P8 证书生成 JWT
	// 这里返回配置的 token 或生成新的
	if s.apnsToken != "" && time.Now().Before(s.apnsTokenExpiry) {
		return s.apnsToken, nil
	}

	// TODO: 使用 P8 证书生成 JWT token
	// 需要：ES256 签名、Kid、Iss、Iat
	return "", fmt.Errorf("APNs token generation not implemented - configure pre-generated token")
}

// getPushType 获取 APNs 推送类型。
func (s *PushSender) getPushType(payload *PushPayload) string {
	if payload.Silent {
		return "background"
	}
	return "alert"
}

// sendFCM 通过 FCM 发送推送。
func (s *PushSender) sendFCM(ctx context.Context, deviceToken string, payload *PushPayload) (string, error) {
	endpoint := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", s.config.FCMProjectID)

	// 构建 FCM 消息
	message := map[string]any{
		"token": deviceToken,
	}

	if !payload.Silent {
		notification := map[string]string{
			"title": payload.Title,
			"body":  payload.Body,
		}
		if payload.ImageURL != "" {
			notification["image"] = payload.ImageURL
		}
		message["notification"] = notification
	}

	// Android 配置
	android := map[string]any{}
	if payload.Priority == "high" {
		android["priority"] = "HIGH"
	} else {
		android["priority"] = "NORMAL"
	}
	if payload.TTL > 0 {
		android["ttl"] = fmt.Sprintf("%ds", payload.TTL)
	}
	if payload.CollapseKey != "" {
		android["collapse_key"] = payload.CollapseKey
	}

	androidNotification := map[string]any{}
	if payload.Sound != "" {
		androidNotification["sound"] = payload.Sound
	}
	if payload.ClickAction != "" {
		androidNotification["click_action"] = payload.ClickAction
	}
	if payload.ChannelID != "" {
		androidNotification["channel_id"] = payload.ChannelID
	}
	if len(androidNotification) > 0 {
		android["notification"] = androidNotification
	}
	if len(android) > 0 {
		message["android"] = android
	}

	// 自定义数据
	if len(payload.Data) > 0 {
		message["data"] = payload.Data
	}

	requestBody := map[string]any{
		"message": message,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// OAuth2 认证
	accessToken, err := s.getFCMAccessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get FCM access token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"error"`
		}
		json.Unmarshal(body, &errResp)
		return "", fmt.Errorf("FCM error: status=%d, message=%s", resp.StatusCode, errResp.Error.Message)
	}

	var result struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return result.Name, nil
}

// getFCMAccessToken 获取 FCM OAuth2 访问令牌。
func (s *PushSender) getFCMAccessToken(ctx context.Context) (string, error) {
	// 简化实现，实际需要使用服务账号密钥生成 OAuth2 token
	// 需要：JWT 签名、调用 Google OAuth2 API
	return "", fmt.Errorf("FCM token generation not implemented - configure service account")
}

// maskToken 遮盖设备令牌用于日志。
func (s *PushSender) maskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "****" + token[len(token)-4:]
}
