// Package outbox 提供了事务性离群消息模式的实现，确保数据库更新与消息发送的原子性。
package outbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/wyfcoding/pkg/tracing"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var defaultManager *Manager

// Default 返回全局默认管理器实例
func Default() *Manager {
	return defaultManager
}

// SetDefault 设置全局默认管理器实例
func SetDefault(m *Manager) {
	defaultManager = m
}

// MessageStatus 消息状态
type MessageStatus int8

const (
	StatusPending MessageStatus = iota // 待发送
	StatusSent                         // 已发送
	StatusFailed                       // 发送失败（超过最大重试次数）
)

// OutboxMessage 离群消息实体模型
type OutboxMessage struct {
	gorm.Model
	Topic      string        `gorm:"column:topic;type:varchar(255);not null;index" json:"topic"` // 消息主题
	Key        string        `gorm:"column:key;type:varchar(255);index" json:"key"`              // 消息键
	Payload    []byte        `gorm:"column:payload;type:blob;not null" json:"payload"`           // 消息体
	Metadata   string        `gorm:"column:metadata;type:text" json:"metadata"`                  // 存储 Trace 上下文 (JSON)
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

// Pusher 消息推送接口
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
func (m *Manager) PublishInTx(ctx context.Context, tx *gorm.DB, topic string, key string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	metadataMap := tracing.InjectContext(ctx)
	metadataJSON, _ := json.Marshal(metadataMap)

	msg := &OutboxMessage{
		Topic:     topic,
		Key:       key,
		Payload:   data,
		Metadata:  string(metadataJSON),
		Status:    StatusPending,
		NextRetry: time.Now(),
	}

	if err := tx.Create(msg).Error; err != nil {
		m.logger.ErrorContext(ctx, "failed to save outbox message", "topic", topic, "error", err)
		return err
	}

	return nil
}

// Processor 离群消息处理器，负责异步发送消息与自清理
type Processor struct {
	mgr           *Manager
	pusher        func(ctx context.Context, topic string, key string, payload []byte) error
	batchSize     int
	interval      time.Duration
	retentionDays int // [新增] 数据保留天数
	stopChan      chan struct{}
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
		mgr:           mgr,
		pusher:        pusher,
		batchSize:     batchSize,
		interval:      interval,
		retentionDays: 7, // 默认保留 7 天
		stopChan:      make(chan struct{}),
	}
}

// Start 启动后台任务
func (p *Processor) Start() {
	p.mgr.logger.Info("outbox processor started")
	ticker := time.NewTicker(p.interval)
	cleanupTicker := time.NewTicker(24 * time.Hour) // 每天清理一次

	go func() {
		for {
			select {
			case <-ticker.C:
				p.process()
			case <-cleanupTicker.C:
				p.cleanup()
			case <-p.stopChan:
				ticker.Stop()
				cleanupTicker.Stop()
				return
			}
		}
	}()
}

// Stop 停止处理器
func (p *Processor) Stop() {
	close(p.stopChan)
}

// process 执行消息投递 (带集群安全并发控制)
func (p *Processor) process() {
	// 核心架构决策：使用事务 + SKIP LOCKED 确保多实例不抢占同一批消息
	// 这样即便有 100 个节点在扫描，它们也不会抓取重复的 ID。
	err := p.mgr.db.Transaction(func(tx *gorm.DB) error {
		var messages []OutboxMessage

		// 1. 抓取待处理消息并加锁
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND next_retry <= ? AND retry_count < max_retries", StatusPending, time.Now()).
			Limit(p.batchSize).
			Order("id ASC").
			Find(&messages).Error
		if err != nil {
			return err
		}

		if len(messages) == 0 {
			return nil
		}

		// 2. 依次投递
		for _, msg := range messages {
			p.send(tx, msg)
		}
		return nil
	})
	if err != nil {
		p.mgr.logger.Error("outbox batch process failed", "error", err)
	}
}

// send 执行单条消息发送并更新状态 (在事务内执行状态更新以保证一致性)
func (p *Processor) send(tx *gorm.DB, msg OutboxMessage) {
	ctx := context.Background()
	if msg.Metadata != "" {
		var carrier map[string]string
		if err := json.Unmarshal([]byte(msg.Metadata), &carrier); err == nil {
			ctx = tracing.ExtractContext(ctx, carrier)
		}
	}

	ctx, span := tracing.StartSpan(ctx, "Outbox.Send")
	defer span.End()

	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	err := p.pusher(sendCtx, msg.Topic, msg.Key, msg.Payload)

	updates := map[string]any{
		"retry_count": msg.RetryCount + 1,
	}

	if err == nil {
		updates["status"] = StatusSent
	} else {
		backoff := min(time.Duration(1<<uint(msg.RetryCount))*time.Minute, 24*time.Hour)
		updates["next_retry"] = time.Now().Add(backoff)
		updates["last_error"] = err.Error()

		if msg.RetryCount+1 >= msg.MaxRetries {
			updates["status"] = StatusFailed
			p.mgr.logger.ErrorContext(ctx, "outbox message failed permanently", "id", msg.ID, "topic", msg.Topic)
		} else {
			p.mgr.logger.WarnContext(ctx, "outbox message send failed, retrying", "id", msg.ID, "error", err)
		}
	}

	// 更新消息状态 (注意此处使用外部传入的事务 tx)
	tx.Model(&msg).Updates(updates)
}

// cleanup 定期清理旧数据，防止表膨胀
func (p *Processor) cleanup() {
	deadline := time.Now().AddDate(0, 0, -p.retentionDays)
	result := p.mgr.db.Where("status = ? AND updated_at < ?", StatusSent, deadline).Delete(&OutboxMessage{})
	if result.Error != nil {
		p.mgr.logger.Error("failed to cleanup old outbox messages", "error", result.Error)
	} else if result.RowsAffected > 0 {
		p.mgr.logger.Info("outbox cleanup finished", "rows_deleted", result.RowsAffected)
	}
}
