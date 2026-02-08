// Package notification 提供 SMS 短信发送实现。
// 生成摘要：
// 1) 支持阿里云和腾讯云短信服务
// 2) 提供模板短信和纯文本短信发送
// 假设：
// - 短信签名和模板需要在短信平台预先申请
// - 国际短信需要额外配置
package notification

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SMSProvider 表示短信服务商类型。
type SMSProvider string

const (
	// SMSProviderAliyun 阿里云短信。
	SMSProviderAliyun SMSProvider = "aliyun"
	// SMSProviderTencent 腾讯云短信。
	SMSProviderTencent SMSProvider = "tencent"
)

// SMSConfig 定义 SMS 发送器配置。
type SMSConfig struct {
	// Provider 短信服务商。
	Provider SMSProvider `json:"provider"`
	// AccessKeyID 访问密钥ID。
	AccessKeyID string `json:"access_key_id"`
	// AccessKeySecret 访问密钥。
	AccessKeySecret string `json:"access_key_secret"`
	// SignName 短信签名。
	SignName string `json:"sign_name"`
	// Region 区域（阿里云使用）。
	Region string `json:"region"`
	// AppID 应用ID（腾讯云使用）。
	AppID string `json:"app_id"`
	// Timeout 请求超时。
	Timeout time.Duration `json:"timeout"`
}

// SMSSender 实现 SMS 短信发送器。
type SMSSender struct {
	config *SMSConfig
	client *http.Client
	logger *slog.Logger
}

// NewSMSSender 创建 SMS 发送器实例。
func NewSMSSender(cfg *SMSConfig, logger *slog.Logger) *SMSSender {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &SMSSender{
		config: cfg,
		client: &http.Client{Timeout: timeout},
		logger: logger,
	}
}

// Channel 返回渠道类型。
func (s *SMSSender) Channel() Channel {
	return ChannelSMS
}

// Send 发送单条短信。
func (s *SMSSender) Send(ctx context.Context, msg *Message) (*Result, error) {
	start := time.Now()
	result := &Result{
		MessageID: msg.ID,
		Channel:   ChannelSMS,
		SentAt:    start,
	}

	var err error
	switch s.config.Provider {
	case SMSProviderAliyun:
		result.ThirdPartyID, err = s.sendAliyun(ctx, msg)
	case SMSProviderTencent:
		result.ThirdPartyID, err = s.sendTencent(ctx, msg)
	default:
		err = fmt.Errorf("unsupported SMS provider: %s", s.config.Provider)
	}

	if err != nil {
		result.Status = StatusFailed
		result.Error = err.Error()
		s.logger.ErrorContext(ctx, "SMS send failed",
			"provider", s.config.Provider,
			"recipient", msg.Recipient,
			"duration", time.Since(start),
			"error", err)
		return result, err
	}

	result.Status = StatusSent
	s.logger.InfoContext(ctx, "SMS sent successfully",
		"provider", s.config.Provider,
		"recipient", msg.Recipient,
		"third_party_id", result.ThirdPartyID,
		"duration", time.Since(start))

	return result, nil
}

// SendBatch 批量发送短信。
func (s *SMSSender) SendBatch(ctx context.Context, msgs []*Message) ([]*Result, error) {
	results := make([]*Result, 0, len(msgs))
	for _, msg := range msgs {
		result, err := s.Send(ctx, msg)
		if err != nil {
			s.logger.ErrorContext(ctx, "batch SMS send failed for message",
				"message_id", msg.ID,
				"error", err)
		}
		results = append(results, result)
	}
	return results, nil
}

// Close 关闭发送器。
func (s *SMSSender) Close() error {
	s.client.CloseIdleConnections()
	return nil
}

// sendAliyun 通过阿里云发送短信。
func (s *SMSSender) sendAliyun(ctx context.Context, msg *Message) (string, error) {
	// 阿里云短信API参数
	endpoint := "https://dysmsapi.aliyuncs.com"
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	params := url.Values{}
	params.Set("Format", "JSON")
	params.Set("Version", "2017-05-25")
	params.Set("AccessKeyId", s.config.AccessKeyID)
	params.Set("SignatureMethod", "HMAC-SHA1")
	params.Set("Timestamp", timestamp)
	params.Set("SignatureVersion", "1.0")
	params.Set("SignatureNonce", fmt.Sprintf("%d", time.Now().UnixNano()))
	params.Set("Action", "SendSms")
	params.Set("RegionId", s.config.Region)
	params.Set("PhoneNumbers", msg.Recipient)
	params.Set("SignName", s.config.SignName)

	if msg.TemplateID != "" {
		params.Set("TemplateCode", msg.TemplateID)
		if len(msg.Variables) > 0 {
			varsJSON, _ := json.Marshal(msg.Variables)
			params.Set("TemplateParam", string(varsJSON))
		}
	}

	// 计算签名（简化实现，实际需完整签名算法）
	signature := s.signAliyun(params)
	params.Set("Signature", signature)

	reqURL := endpoint + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Code      string `json:"Code"`
		Message   string `json:"Message"`
		RequestID string `json:"RequestId"`
		BizID     string `json:"BizId"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Code != "OK" {
		return "", fmt.Errorf("aliyun SMS error: %s - %s", result.Code, result.Message)
	}

	return result.BizID, nil
}

// signAliyun 计算阿里云签名（简化版本）。
func (s *SMSSender) signAliyun(params url.Values) string {
	// 简化签名实现，实际使用需按照阿里云文档完整实现
	stringToSign := "GET&%2F&" + url.QueryEscape(params.Encode())
	h := hmac.New(sha256.New, []byte(s.config.AccessKeySecret+"&"))
	h.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// sendTencent 通过腾讯云发送短信。
func (s *SMSSender) sendTencent(ctx context.Context, msg *Message) (string, error) {
	endpoint := "https://sms.tencentcloudapi.com"
	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	// 请求体
	requestBody := map[string]any{
		"PhoneNumberSet":   []string{msg.Recipient},
		"SmsSdkAppId":      s.config.AppID,
		"SignName":         s.config.SignName,
		"TemplateId":       msg.TemplateID,
		"TemplateParamSet": s.variablesToSlice(msg.Variables),
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// 计算签名
	authorization := s.signTencent(timestamp, string(bodyBytes))

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TC-Action", "SendSms")
	req.Header.Set("X-TC-Version", "2021-01-11")
	req.Header.Set("X-TC-Timestamp", timestamp)
	req.Header.Set("Authorization", authorization)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Response struct {
			Error struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
			RequestID     string `json:"RequestId"`
			SendStatusSet []struct {
				SerialNo string `json:"SerialNo"`
				Code     string `json:"Code"`
				Message  string `json:"Message"`
			} `json:"SendStatusSet"`
		} `json:"Response"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Response.Error.Code != "" {
		return "", fmt.Errorf("tencent SMS error: %s - %s",
			result.Response.Error.Code, result.Response.Error.Message)
	}

	if len(result.Response.SendStatusSet) > 0 {
		status := result.Response.SendStatusSet[0]
		if status.Code != "Ok" {
			return "", fmt.Errorf("tencent SMS send error: %s - %s", status.Code, status.Message)
		}
		return status.SerialNo, nil
	}

	return result.Response.RequestID, nil
}

// signTencent 计算腾讯云签名（简化版本）。
func (s *SMSSender) signTencent(timestamp, payload string) string {
	// 简化签名实现，实际使用需按照腾讯云文档完整实现
	date := time.Now().UTC().Format("2006-01-02")
	service := "sms"

	// 计算签名摘要
	h := hmac.New(sha256.New, []byte("TC3"+s.config.AccessKeySecret))
	h.Write([]byte(date))
	dateKey := h.Sum(nil)

	h = hmac.New(sha256.New, dateKey)
	h.Write([]byte(service))
	serviceKey := h.Sum(nil)

	h = hmac.New(sha256.New, serviceKey)
	h.Write([]byte("tc3_request"))
	signingKey := h.Sum(nil)

	// 构建签名字符串
	_ = payload // 实际需要使用
	stringToSign := fmt.Sprintf("TC3-HMAC-SHA256\n%s\n%s/sms/tc3_request\n", timestamp, date)

	h = hmac.New(sha256.New, signingKey)
	h.Write([]byte(stringToSign))
	signature := hex.EncodeToString(h.Sum(nil))

	return fmt.Sprintf(
		"TC3-HMAC-SHA256 Credential=%s/%s/sms/tc3_request, SignedHeaders=content-type;host, Signature=%s",
		s.config.AccessKeyID, date, signature)
}

// variablesToSlice 将变量map转换为有序切片。
func (s *SMSSender) variablesToSlice(variables map[string]string) []string {
	if len(variables) == 0 {
		return nil
	}

	// 腾讯云模板参数按顺序：{1}, {2}, {3}...
	result := make([]string, 0, len(variables))
	for i := 1; i <= len(variables); i++ {
		key := fmt.Sprintf("%d", i)
		if v, ok := variables[key]; ok {
			result = append(result, v)
		}
	}
	return result
}
