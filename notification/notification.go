// Package notification 提供统一的多渠道通知服务封装。
// 生成摘要：
// 1) 支持 SMS/Email/Push/IM 多渠道通知
// 2) 提供模板管理、通知记录、重试机制
// 3) 支持异步发送和批量发送
// 假设：
// - 各渠道具体实现需要配置对应的第三方服务凭证
// - 模板变量使用 {{.FieldName}} 格式
package notification

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Channel 表示通知渠道类型。
type Channel string

const (
	// ChannelSMS 短信渠道。
	ChannelSMS Channel = "sms"
	// ChannelEmail 邮件渠道。
	ChannelEmail Channel = "email"
	// ChannelPush App推送渠道。
	ChannelPush Channel = "push"
	// ChannelIM 即时消息渠道（如企业微信、钉钉）。
	ChannelIM Channel = "im"
	// ChannelWebhook Webhook回调渠道。
	ChannelWebhook Channel = "webhook"
)

// Status 表示通知发送状态。
type Status string

const (
	// StatusPending 待发送。
	StatusPending Status = "pending"
	// StatusSending 发送中。
	StatusSending Status = "sending"
	// StatusSent 已发送。
	StatusSent Status = "sent"
	// StatusFailed 发送失败。
	StatusFailed Status = "failed"
	// StatusRetrying 重试中。
	StatusRetrying Status = "retrying"
)

// Priority 表示通知优先级。
type Priority int

const (
	// PriorityLow 低优先级。
	PriorityLow Priority = 1
	// PriorityNormal 普通优先级。
	PriorityNormal Priority = 2
	// PriorityHigh 高优先级。
	PriorityHigh Priority = 3
	// PriorityUrgent 紧急优先级。
	PriorityUrgent Priority = 4
)

// Message 表示一条待发送的通知消息。
type Message struct {
	// ID 消息唯一标识。
	ID string `json:"id"`
	// Channel 通知渠道。
	Channel Channel `json:"channel"`
	// TemplateID 模板ID，如果使用模板发送。
	TemplateID string `json:"template_id,omitempty"`
	// Recipient 接收者标识（手机号/邮箱/设备Token/用户ID）。
	Recipient string `json:"recipient"`
	// Subject 消息主题（邮件/Push标题）。
	Subject string `json:"subject,omitempty"`
	// Content 消息内容。
	Content string `json:"content"`
	// Variables 模板变量。
	Variables map[string]string `json:"variables,omitempty"`
	// Priority 优先级。
	Priority Priority `json:"priority"`
	// Metadata 扩展元数据。
	Metadata map[string]string `json:"metadata,omitempty"`
	// ScheduledAt 定时发送时间，为空则立即发送。
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
	// ExpireAt 消息过期时间。
	ExpireAt *time.Time `json:"expire_at,omitempty"`
}

// Result 表示通知发送结果。
type Result struct {
	// MessageID 消息ID。
	MessageID string `json:"message_id"`
	// Channel 发送渠道。
	Channel Channel `json:"channel"`
	// Status 发送状态。
	Status Status `json:"status"`
	// ThirdPartyID 第三方平台返回的消息ID。
	ThirdPartyID string `json:"third_party_id,omitempty"`
	// Error 错误信息。
	Error string `json:"error,omitempty"`
	// SentAt 发送时间。
	SentAt time.Time `json:"sent_at"`
	// RetryCount 重试次数。
	RetryCount int `json:"retry_count"`
}

// Record 表示通知记录（用于持久化）。
type Record struct {
	ID           string            `json:"id"`
	Channel      Channel           `json:"channel"`
	TemplateID   string            `json:"template_id,omitempty"`
	Recipient    string            `json:"recipient"`
	Subject      string            `json:"subject,omitempty"`
	Content      string            `json:"content"`
	Variables    map[string]string `json:"variables,omitempty"`
	Status       Status            `json:"status"`
	ThirdPartyID string            `json:"third_party_id,omitempty"`
	Error        string            `json:"error,omitempty"`
	Priority     Priority          `json:"priority"`
	RetryCount   int               `json:"retry_count"`
	MaxRetries   int               `json:"max_retries"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	SentAt       *time.Time        `json:"sent_at,omitempty"`
	ScheduledAt  *time.Time        `json:"scheduled_at,omitempty"`
}

// Sender 定义渠道发送器接口。
type Sender interface {
	// Channel 返回该发送器支持的渠道。
	Channel() Channel
	// Send 发送单条消息。
	Send(ctx context.Context, msg *Message) (*Result, error)
	// SendBatch 批量发送消息。
	SendBatch(ctx context.Context, msgs []*Message) ([]*Result, error)
	// Close 关闭发送器，释放资源。
	Close() error
}

// TemplateRenderer 定义模板渲染接口。
type TemplateRenderer interface {
	// Render 渲染模板，返回渲染后的内容。
	Render(ctx context.Context, templateID string, variables map[string]string) (subject, content string, err error)
}

// RecordRepository 定义通知记录存储接口。
type RecordRepository interface {
	// Save 保存通知记录。
	Save(ctx context.Context, record *Record) error
	// Get 获取通知记录。
	Get(ctx context.Context, id string) (*Record, error)
	// ListPending 获取待发送的记录。
	ListPending(ctx context.Context, limit int) ([]*Record, error)
	// ListFailed 获取失败的记录（用于重试）。
	ListFailed(ctx context.Context, maxRetries int, limit int) ([]*Record, error)
	// Update 更新通知记录。
	Update(ctx context.Context, record *Record) error
}

// Config 定义通知服务配置。
type Config struct {
	// DefaultMaxRetries 默认最大重试次数。
	DefaultMaxRetries int `json:"default_max_retries"`
	// RetryInterval 重试间隔。
	RetryInterval time.Duration `json:"retry_interval"`
	// AsyncWorkers 异步发送工作协程数。
	AsyncWorkers int `json:"async_workers"`
	// QueueSize 异步队列大小。
	QueueSize int `json:"queue_size"`
}

// DefaultConfig 返回默认配置。
func DefaultConfig() *Config {
	return &Config{
		DefaultMaxRetries: 3,
		RetryInterval:     time.Minute,
		AsyncWorkers:      5,
		QueueSize:         1000,
	}
}

// Service 统一通知服务。
type Service struct {
	senders    map[Channel]Sender
	renderer   TemplateRenderer
	repository RecordRepository
	config     *Config
	logger     *slog.Logger

	// 异步发送支持
	asyncQueue chan *Message
	wg         sync.WaitGroup
	stopCh     chan struct{}
	mu         sync.RWMutex
}

// NewService 创建通知服务实例。
func NewService(cfg *Config, logger *slog.Logger) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		senders:    make(map[Channel]Sender),
		config:     cfg,
		logger:     logger,
		asyncQueue: make(chan *Message, cfg.QueueSize),
		stopCh:     make(chan struct{}),
	}
}

// RegisterSender 注册渠道发送器。
func (s *Service) RegisterSender(sender Sender) {
	if sender == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.senders[sender.Channel()] = sender
	s.logger.Info("notification sender registered", "channel", sender.Channel())
}

// SetTemplateRenderer 设置模板渲染器。
func (s *Service) SetTemplateRenderer(renderer TemplateRenderer) {
	s.renderer = renderer
}

// SetRepository 设置记录存储。
func (s *Service) SetRepository(repo RecordRepository) {
	s.repository = repo
}

// Start 启动异步发送工作协程。
func (s *Service) Start() {
	for i := 0; i < s.config.AsyncWorkers; i++ {
		s.wg.Add(1)
		go s.asyncWorker(i)
	}
	s.logger.Info("notification service started", "workers", s.config.AsyncWorkers)
}

// Stop 停止服务并等待所有任务完成。
func (s *Service) Stop() {
	close(s.stopCh)
	s.wg.Wait()

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sender := range s.senders {
		if err := sender.Close(); err != nil {
			s.logger.Error("failed to close sender", "channel", sender.Channel(), "error", err)
		}
	}
	s.logger.Info("notification service stopped")
}

// Send 同步发送通知。
func (s *Service) Send(ctx context.Context, msg *Message) (*Result, error) {
	if msg == nil {
		return nil, fmt.Errorf("notification message is nil")
	}
	start := time.Now()

	s.mu.RLock()
	sender, ok := s.senders[msg.Channel]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no sender registered for channel: %s", msg.Channel)
	}

	// 模板渲染
	if msg.TemplateID != "" && s.renderer != nil {
		subject, content, err := s.renderer.Render(ctx, msg.TemplateID, msg.Variables)
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to render template",
				"template_id", msg.TemplateID,
				"error", err)
			return nil, fmt.Errorf("failed to render template: %w", err)
		}
		if subject != "" {
			msg.Subject = subject
		}
		msg.Content = content
	}

	// 发送消息
	result, err := sender.Send(ctx, msg)
	duration := time.Since(start)

	// 记录日志
	if err != nil {
		s.logger.ErrorContext(ctx, "notification send failed",
			"channel", msg.Channel,
			"recipient", msg.Recipient,
			"duration", duration,
			"error", err)
	} else {
		s.logger.InfoContext(ctx, "notification sent successfully",
			"channel", msg.Channel,
			"recipient", msg.Recipient,
			"message_id", result.MessageID,
			"duration", duration)
	}

	// 持久化记录
	if s.repository != nil {
		record := s.messageToRecord(msg, result, err)
		if saveErr := s.repository.Save(ctx, record); saveErr != nil {
			s.logger.ErrorContext(ctx, "failed to save notification record",
				"message_id", msg.ID,
				"error", saveErr)
		}
	}

	return result, err
}

// SendAsync 异步发送通知。
func (s *Service) SendAsync(ctx context.Context, msg *Message) error {
	if msg == nil {
		return fmt.Errorf("notification message is nil")
	}
	select {
	case s.asyncQueue <- msg:
		s.logger.DebugContext(ctx, "notification queued for async send",
			"channel", msg.Channel,
			"recipient", msg.Recipient)
		return nil
	default:
		return fmt.Errorf("notification queue is full")
	}
}

// SendBatch 批量发送通知。
func (s *Service) SendBatch(ctx context.Context, msgs []*Message) ([]*Result, error) {
	if len(msgs) == 0 {
		return nil, nil
	}

	// 按渠道分组
	channelMsgs := make(map[Channel][]*Message)
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		channelMsgs[msg.Channel] = append(channelMsgs[msg.Channel], msg)
	}

	var allResults []*Result
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 并行按渠道发送
	for channel, channelMessages := range channelMsgs {
		wg.Add(1)
		go func(ch Channel, cMsgs []*Message) {
			defer wg.Done()

			s.mu.RLock()
			sender, ok := s.senders[ch]
			s.mu.RUnlock()

			if !ok {
				s.logger.ErrorContext(ctx, "no sender for channel", "channel", ch)
				return
			}

			results, err := sender.SendBatch(ctx, cMsgs)
			if err != nil {
				s.logger.ErrorContext(ctx, "batch send failed",
					"channel", ch,
					"count", len(cMsgs),
					"error", err)
				return
			}

			mu.Lock()
			allResults = append(allResults, results...)
			mu.Unlock()
		}(channel, channelMessages)
	}

	wg.Wait()
	return allResults, nil
}

// asyncWorker 异步发送工作协程。
func (s *Service) asyncWorker(id int) {
	defer s.wg.Done()

	for {
		select {
		case <-s.stopCh:
			s.logger.Debug("async worker stopping", "worker_id", id)
			return
		case msg := <-s.asyncQueue:
			ctx := context.Background()
			if _, err := s.Send(ctx, msg); err != nil {
				s.logger.ErrorContext(ctx, "async send failed",
					"worker_id", id,
					"channel", msg.Channel,
					"recipient", msg.Recipient,
					"error", err)
			}
		}
	}
}

// messageToRecord 将消息转换为记录。
func (s *Service) messageToRecord(msg *Message, result *Result, err error) *Record {
	now := time.Now()
	record := &Record{
		ID:          msg.ID,
		Channel:     msg.Channel,
		TemplateID:  msg.TemplateID,
		Recipient:   msg.Recipient,
		Subject:     msg.Subject,
		Content:     msg.Content,
		Variables:   msg.Variables,
		Priority:    msg.Priority,
		MaxRetries:  s.config.DefaultMaxRetries,
		CreatedAt:   now,
		UpdatedAt:   now,
		ScheduledAt: msg.ScheduledAt,
	}

	if err != nil {
		record.Status = StatusFailed
		record.Error = err.Error()
	} else if result != nil {
		record.Status = result.Status
		record.ThirdPartyID = result.ThirdPartyID
		record.RetryCount = result.RetryCount
		record.SentAt = &result.SentAt
	}

	return record
}

// RetryFailed 重试失败的通知。
func (s *Service) RetryFailed(ctx context.Context, limit int) (int, error) {
	if s.repository == nil {
		return 0, fmt.Errorf("repository not configured")
	}

	records, err := s.repository.ListFailed(ctx, s.config.DefaultMaxRetries, limit)
	if err != nil {
		return 0, fmt.Errorf("failed to list failed records: %w", err)
	}

	retryCount := 0
	for _, record := range records {
		msg := &Message{
			ID:          record.ID,
			Channel:     record.Channel,
			TemplateID:  record.TemplateID,
			Recipient:   record.Recipient,
			Subject:     record.Subject,
			Content:     record.Content,
			Variables:   record.Variables,
			Priority:    record.Priority,
			ScheduledAt: record.ScheduledAt,
		}

		record.Status = StatusRetrying
		record.RetryCount++
		record.UpdatedAt = time.Now()

		if updateErr := s.repository.Update(ctx, record); updateErr != nil {
			s.logger.ErrorContext(ctx, "failed to update record status",
				"record_id", record.ID,
				"error", updateErr)
			continue
		}

		if _, err := s.Send(ctx, msg); err != nil {
			s.logger.ErrorContext(ctx, "retry send failed",
				"record_id", record.ID,
				"retry_count", record.RetryCount,
				"error", err)
		} else {
			retryCount++
		}
	}

	return retryCount, nil
}
