// Package notification 提供 Email 邮件发送实现。
// 生成摘要：
// 1) 支持 SMTP 和 API 两种发送方式
// 2) 支持 HTML 和纯文本邮件
// 3) 支持附件和内嵌图片
// 假设：
// - SMTP 服务器需要支持 TLS
// - API 方式目前支持 SendGrid
package notification

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/smtp"
	"net/textproto"
	"path/filepath"
	"strings"
	"time"
)

// EmailProvider 表示邮件服务商类型。
type EmailProvider string

const (
	// EmailProviderSMTP 使用 SMTP 协议发送。
	EmailProviderSMTP EmailProvider = "smtp"
	// EmailProviderSendGrid 使用 SendGrid API 发送。
	EmailProviderSendGrid EmailProvider = "sendgrid"
)

// EmailConfig 定义 Email 发送器配置。
type EmailConfig struct {
	// Provider 邮件服务商。
	Provider EmailProvider `json:"provider"`
	// SMTPHost SMTP 服务器地址。
	SMTPHost string `json:"smtp_host"`
	// SMTPPort SMTP 服务器端口。
	SMTPPort int `json:"smtp_port"`
	// Username SMTP 用户名。
	Username string `json:"username"`
	// Password SMTP 密码或授权码。
	Password string `json:"password"`
	// FromAddress 发件人地址。
	FromAddress string `json:"from_address"`
	// FromName 发件人名称。
	FromName string `json:"from_name"`
	// UseTLS 是否使用 TLS。
	UseTLS bool `json:"use_tls"`
	// APIKey API 密钥（用于 SendGrid 等）。
	APIKey string `json:"api_key"`
	// Timeout 请求超时。
	Timeout time.Duration `json:"timeout"`
}

// EmailAttachment 表示邮件附件。
type EmailAttachment struct {
	// Filename 附件文件名。
	Filename string `json:"filename"`
	// Content 附件内容（Base64编码）。
	Content string `json:"content"`
	// ContentType 内容类型。
	ContentType string `json:"content_type"`
	// Inline 是否内嵌（用于图片）。
	Inline bool `json:"inline"`
	// ContentID 内嵌内容ID。
	ContentID string `json:"content_id,omitempty"`
}

// EmailMessage 扩展的邮件消息。
type EmailMessage struct {
	*Message
	// To 收件人列表。
	To []string `json:"to"`
	// CC 抄送列表。
	CC []string `json:"cc,omitempty"`
	// BCC 密送列表。
	BCC []string `json:"bcc,omitempty"`
	// ReplyTo 回复地址。
	ReplyTo string `json:"reply_to,omitempty"`
	// HTMLContent HTML 内容。
	HTMLContent string `json:"html_content,omitempty"`
	// Attachments 附件列表。
	Attachments []*EmailAttachment `json:"attachments,omitempty"`
}

// EmailSender 实现 Email 邮件发送器。
type EmailSender struct {
	config *EmailConfig
	client *http.Client
	logger *slog.Logger
}

// NewEmailSender 创建 Email 发送器实例。
func NewEmailSender(cfg *EmailConfig, logger *slog.Logger) *EmailSender {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &EmailSender{
		config: cfg,
		client: &http.Client{Timeout: timeout},
		logger: logger,
	}
}

// Channel 返回渠道类型。
func (s *EmailSender) Channel() Channel {
	return ChannelEmail
}

// Send 发送单封邮件。
func (s *EmailSender) Send(ctx context.Context, msg *Message) (*Result, error) {
	start := time.Now()
	result := &Result{
		MessageID: msg.ID,
		Channel:   ChannelEmail,
		SentAt:    start,
	}

	// 构建扩展邮件消息
	emailMsg := &EmailMessage{
		Message: msg,
		To:      []string{msg.Recipient},
	}

	// 检查是否有 HTML 内容在 metadata 中
	if msg.Metadata != nil {
		if html, ok := msg.Metadata["html_content"]; ok {
			emailMsg.HTMLContent = html
		}
	}

	var err error
	switch s.config.Provider {
	case EmailProviderSMTP:
		err = s.sendSMTP(ctx, emailMsg)
	case EmailProviderSendGrid:
		result.ThirdPartyID, err = s.sendSendGrid(ctx, emailMsg)
	default:
		err = fmt.Errorf("unsupported email provider: %s", s.config.Provider)
	}

	if err != nil {
		result.Status = StatusFailed
		result.Error = err.Error()
		s.logger.ErrorContext(ctx, "email send failed",
			"provider", s.config.Provider,
			"recipient", msg.Recipient,
			"duration", time.Since(start),
			"error", err)
		return result, err
	}

	result.Status = StatusSent
	s.logger.InfoContext(ctx, "email sent successfully",
		"provider", s.config.Provider,
		"recipient", msg.Recipient,
		"subject", msg.Subject,
		"duration", time.Since(start))

	return result, nil
}

// SendBatch 批量发送邮件。
func (s *EmailSender) SendBatch(ctx context.Context, msgs []*Message) ([]*Result, error) {
	results := make([]*Result, 0, len(msgs))
	for _, msg := range msgs {
		result, err := s.Send(ctx, msg)
		if err != nil {
			s.logger.ErrorContext(ctx, "batch email send failed for message",
				"message_id", msg.ID,
				"error", err)
		}
		results = append(results, result)
	}
	return results, nil
}

// Close 关闭发送器。
func (s *EmailSender) Close() error {
	s.client.CloseIdleConnections()
	return nil
}

// sendSMTP 通过 SMTP 发送邮件。
func (s *EmailSender) sendSMTP(_ context.Context, msg *EmailMessage) error {
	// 构建邮件内容
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// 邮件头
	headers := make(textproto.MIMEHeader)
	headers.Set("From", s.formatAddress(s.config.FromName, s.config.FromAddress))
	headers.Set("To", strings.Join(msg.To, ", "))
	if len(msg.CC) > 0 {
		headers.Set("Cc", strings.Join(msg.CC, ", "))
	}
	headers.Set("Subject", s.encodeSubject(msg.Subject))
	headers.Set("MIME-Version", "1.0")
	headers.Set("Content-Type", fmt.Sprintf("multipart/mixed; boundary=%s", writer.Boundary()))
	headers.Set("Date", time.Now().Format(time.RFC1123Z))
	headers.Set("Message-ID", fmt.Sprintf("<%s@%s>", msg.ID, s.config.SMTPHost))

	// 写入头部
	var headerBuf bytes.Buffer
	for key, values := range headers {
		for _, value := range values {
			fmt.Fprintf(&headerBuf, "%s: %s\r\n", key, value)
		}
	}
	fmt.Fprint(&headerBuf, "\r\n")

	// 邮件正文部分
	if msg.HTMLContent != "" {
		// HTML 和纯文本混合
		altWriter := multipart.NewWriter(&buf)
		altHeader := make(textproto.MIMEHeader)
		altHeader.Set("Content-Type", fmt.Sprintf("multipart/alternative; boundary=%s", altWriter.Boundary()))

		part, _ := writer.CreatePart(altHeader)

		// 纯文本部分
		textHeader := make(textproto.MIMEHeader)
		textHeader.Set("Content-Type", "text/plain; charset=utf-8")
		textHeader.Set("Content-Transfer-Encoding", "base64")
		textPart, _ := altWriter.CreatePart(textHeader)
		textPart.Write([]byte(base64.StdEncoding.EncodeToString([]byte(msg.Content))))

		// HTML 部分
		htmlHeader := make(textproto.MIMEHeader)
		htmlHeader.Set("Content-Type", "text/html; charset=utf-8")
		htmlHeader.Set("Content-Transfer-Encoding", "base64")
		htmlPart, _ := altWriter.CreatePart(htmlHeader)
		htmlPart.Write([]byte(base64.StdEncoding.EncodeToString([]byte(msg.HTMLContent))))

		altWriter.Close()
		part.Write(buf.Bytes())
	} else {
		// 纯文本
		textHeader := make(textproto.MIMEHeader)
		textHeader.Set("Content-Type", "text/plain; charset=utf-8")
		textHeader.Set("Content-Transfer-Encoding", "base64")
		part, _ := writer.CreatePart(textHeader)
		part.Write([]byte(base64.StdEncoding.EncodeToString([]byte(msg.Content))))
	}

	// 附件
	for _, att := range msg.Attachments {
		attHeader := make(textproto.MIMEHeader)
		if att.Inline {
			attHeader.Set("Content-Disposition", fmt.Sprintf("inline; filename=%s", att.Filename))
			if att.ContentID != "" {
				attHeader.Set("Content-ID", fmt.Sprintf("<%s>", att.ContentID))
			}
		} else {
			attHeader.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", att.Filename))
		}
		contentType := att.ContentType
		if contentType == "" {
			contentType = mime.TypeByExtension(filepath.Ext(att.Filename))
			if contentType == "" {
				contentType = "application/octet-stream"
			}
		}
		attHeader.Set("Content-Type", contentType)
		attHeader.Set("Content-Transfer-Encoding", "base64")

		part, _ := writer.CreatePart(attHeader)
		part.Write([]byte(att.Content))
	}

	writer.Close()

	// 合并邮件内容
	emailContent := append(headerBuf.Bytes(), buf.Bytes()...)

	// 发送邮件
	addr := fmt.Sprintf("%s:%d", s.config.SMTPHost, s.config.SMTPPort)
	auth := smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.SMTPHost)

	// 获取所有收件人
	recipients := make([]string, 0, len(msg.To)+len(msg.CC)+len(msg.BCC))
	recipients = append(recipients, msg.To...)
	recipients = append(recipients, msg.CC...)
	recipients = append(recipients, msg.BCC...)

	if s.config.UseTLS {
		return s.sendSMTPWithTLS(addr, auth, s.config.FromAddress, recipients, emailContent)
	}

	return smtp.SendMail(addr, auth, s.config.FromAddress, recipients, emailContent)
}

// sendSMTPWithTLS 使用 TLS 发送邮件。
func (s *EmailSender) sendSMTPWithTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{
		ServerName: s.config.SMTPHost,
	})
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.config.SMTPHost)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("failed to add recipient %s: %w", recipient, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to get data writer: %w", err)
	}

	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}

	return client.Quit()
}

// sendSendGrid 通过 SendGrid API 发送邮件。
func (s *EmailSender) sendSendGrid(ctx context.Context, msg *EmailMessage) (string, error) {
	endpoint := "https://api.sendgrid.com/v3/mail/send"

	// 构建请求体
	requestBody := map[string]any{
		"personalizations": []map[string]any{
			{
				"to": s.formatRecipients(msg.To),
			},
		},
		"from": map[string]string{
			"email": s.config.FromAddress,
			"name":  s.config.FromName,
		},
		"subject": msg.Subject,
		"content": []map[string]string{
			{"type": "text/plain", "value": msg.Content},
		},
	}

	// 添加 CC
	if len(msg.CC) > 0 {
		requestBody["personalizations"].([]map[string]any)[0]["cc"] = s.formatRecipients(msg.CC)
	}

	// 添加 BCC
	if len(msg.BCC) > 0 {
		requestBody["personalizations"].([]map[string]any)[0]["bcc"] = s.formatRecipients(msg.BCC)
	}

	// 添加 HTML 内容
	if msg.HTMLContent != "" {
		requestBody["content"] = []map[string]string{
			{"type": "text/plain", "value": msg.Content},
			{"type": "text/html", "value": msg.HTMLContent},
		}
	}

	// 添加附件
	if len(msg.Attachments) > 0 {
		attachments := make([]map[string]string, 0, len(msg.Attachments))
		for _, att := range msg.Attachments {
			attachment := map[string]string{
				"content":  att.Content,
				"filename": att.Filename,
				"type":     att.ContentType,
			}
			if att.Inline {
				attachment["disposition"] = "inline"
				if att.ContentID != "" {
					attachment["content_id"] = att.ContentID
				}
			} else {
				attachment["disposition"] = "attachment"
			}
			attachments = append(attachments, attachment)
		}
		requestBody["attachments"] = attachments
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
	req.Header.Set("Authorization", "Bearer "+s.config.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("sendgrid error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	// 获取消息ID
	messageID := resp.Header.Get("X-Message-Id")
	return messageID, nil
}

// formatAddress 格式化邮件地址。
func (s *EmailSender) formatAddress(name, email string) string {
	if name == "" {
		return email
	}
	return fmt.Sprintf("%s <%s>", name, email)
}

// formatRecipients 格式化收件人列表。
func (s *EmailSender) formatRecipients(emails []string) []map[string]string {
	result := make([]map[string]string, 0, len(emails))
	for _, email := range emails {
		result = append(result, map[string]string{"email": email})
	}
	return result
}

// encodeSubject 编码邮件主题（支持中文）。
func (s *EmailSender) encodeSubject(subject string) string {
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(subject)) + "?="
}
