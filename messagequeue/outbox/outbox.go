// Package outbox 提供了事务性离群消息模式的实现，确保数据库更新与消息发送的原子性。
package outbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"gorm.io/gorm"
)

// MessageStatus 消息状态
type MessageStatus int8

const (
	StatusPending MessageStatus = iota // 待发送
	StatusSent                         // 已发送
	StatusFailed                       // 发送失败（超过最大重试次数）
)

// OutboxMessage 离群消息实体模型
// 必须与业务表在同一个数据库中，以便利用本地事务。
type OutboxMessage struct {
	gorm.Model
	Topic      string        `gorm:"column:topic;type:varchar(255);not null;index" json:"topic"` // 消息主题
	Key        string        `gorm:"column:key;type:varchar(255);index" json:"key"`              // 消息键 (用于 Kafka 分区)
	Payload    []byte        `gorm:"column:payload;type:blob;not null" json:"payload"`           // 消息体
	Status     MessageStatus `gorm:"column:status;type:tinyint;default:0;index" json:"status"`   // 状态
	RetryCount int           `gorm:"column:retry_count;type:int;default:0" json:"retry_count"`   // 已重试次数
	MaxRetries int           `gorm:"column:max_retries;type:int;default:5" json:"max_retries"`   // 最大重试次数
	NextRetry  time.Time     `gorm:"column:next_retry;type:datetime;index" json:"next_retry"`    // 下次重试时间
	LastError  string        `gorm:"column:last_error;type:text" json:"last_error"`              // 最后一次错误信息
}

// TableName 指定表名
func (OutboxMessage) TableName() string {
	return "sys_outbox_messages"
}

// Pusher 消息推送接口，屏蔽底层 MQ 实现 (如 Kafka, RabbitMQ)
type Pusher interface {
	Push(ctx context.Context, topic string, key string, payload []byte) error
}

// Manager 离群消息管理器
type Manager struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewManager 创建 Outbox 管理器
func NewManager(db *gorm.DB, logger *slog.Logger) *Manager {
	return &Manager{
		db:     db,
		logger: logger.With("module", "outbox"),
	}
}

// PublishInTx 在现有的事务中持久化消息
// 这是 Outbox 模式的核心：将消息存入数据库的操作必须作为业务事务的一部分。
func (m *Manager) PublishInTx(tx *gorm.DB, topic string, key string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	msg := &OutboxMessage{
		Topic:     topic,
		Key:       key,
		Payload:   data,
		Status:    StatusPending,
		NextRetry: time.Now(),
	}

	if err := tx.Create(msg).Error; err != nil {
		m.logger.Error("failed to save outbox message", "topic", topic, "error", err)
		return err
	}

	return nil
}

// Processor 离群消息处理器，负责异步发送消息
type Processor struct {
	mgr       *Manager
	pusher    func(ctx context.Context, topic string, key string, payload []byte) error // 发送函数
	batchSize int
	interval  time.Duration
	stopChan  chan struct{}
}

// NewProcessor 创建处理器
func NewProcessor(mgr *Manager, pusher func(ctx context.Context, topic string, key string, payload []byte) error, batchSize int, interval time.Duration) *Processor {
	if batchSize <= 0 {
		batchSize = 100
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Processor{
		mgr:       mgr,
		pusher:    pusher,
		batchSize: batchSize,
		interval:  interval,
		stopChan:  make(chan struct{}),
	}
}

// Start 启动后台扫描任务
func (p *Processor) Start() {
	p.mgr.logger.Info("outbox processor started")
	ticker := time.NewTicker(p.interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				p.process()
			case <-p.stopChan:
				ticker.Stop()
				return
			}
		}
	}()
}

// Stop 停止处理器
func (p *Processor) Stop() {
	close(p.stopChan)
	p.mgr.logger.Info("outbox processor stopped")
}

// process 执行一次扫描与投递
func (p *Processor) process() {
	var messages []OutboxMessage
	// 查找待处理或重试时间已到的消息
	err := p.mgr.db.Where("status = ? AND next_retry <= ? AND retry_count < max_retries", StatusPending, time.Now()).
		Limit(p.batchSize).
		Order("id ASC").
		Find(&messages).Error
	if err != nil {
		p.mgr.logger.Error("failed to fetch outbox messages", "error", err)
		return
	}

	for _, msg := range messages {
		p.send(msg)
	}
}

// send 执行单条消息发送并更新状态
func (p *Processor) send(msg OutboxMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := p.pusher(ctx, msg.Topic, msg.Key, msg.Payload)
	if err == nil {
		// 发送成功：标记为已发送
		p.mgr.db.Model(&msg).Updates(map[string]any{
			"status":      StatusSent,
			"retry_count": msg.RetryCount + 1,
		})
		return
	}

	// 发送失败：增加重试次数，计算下次重试时间（指数退避策略）
	backoff := min(time.Duration(1<<uint(msg.RetryCount))*time.Minute, 24*time.Hour)

	updates := map[string]any{
		"retry_count": msg.RetryCount + 1,
		"next_retry":  time.Now().Add(backoff),
		"last_error":  err.Error(),
	}

	if msg.RetryCount+1 >= msg.MaxRetries {
		updates["status"] = StatusFailed
		p.mgr.logger.Error("outbox message failed permanently", "id", msg.ID, "topic", msg.Topic)
	} else {
		p.mgr.logger.Warn("outbox message send failed, retrying later", "id", msg.ID, "error", err, "next_retry", updates["next_retry"])
	}

	p.mgr.db.Model(&msg).Updates(updates)
}
