// Package outbox 提供了事务性离群消息模式的实现，确保数据库更新与消息发送的原子性。
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/wyfcoding/pkg/cast"
	"github.com/wyfcoding/pkg/tracing"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var defaultManager *Manager

// Default 返回全局默认管理器实例。
func Default() *Manager {
	return defaultManager
}

// SetDefault 设置全局默认管理器实例。
func SetDefault(manager *Manager) {
	defaultManager = manager
}

// MessageStatus 消息状态。
type MessageStatus int8

const (
	// StatusPending 待发送状态。
	StatusPending MessageStatus = iota
	// StatusSent 已发送状态。
	StatusSent
	// StatusFailed 发送失败（超过最大重试次数）。
	StatusFailed
)

// Message 离群消息实体模型。
type Message struct {
	NextRetry time.Time `gorm:"column:next_retry;type:datetime;index" json:"next_retry"` // 下次重试时间。
	gorm.Model
	Topic      string        `gorm:"column:topic;type:varchar(255);not null;index" json:"topic"` // 消息主题。
	Key        string        `gorm:"column:key;type:varchar(255);index" json:"key"`              // 消息键。
	Metadata   string        `gorm:"column:metadata;type:text" json:"metadata"`                  // 存储 Trace 上下文 (JSON)。
	LastError  string        `gorm:"column:last_error;type:text" json:"last_error"`              // 最后一次错误信息。
	Payload    []byte        `gorm:"column:payload;type:blob;not null" json:"payload"`           // 消息体。
	RetryCount int           `gorm:"column:retry_count;type:int;default:0" json:"retry_count"`   // 已重试次数。
	MaxRetries int           `gorm:"column:max_retries;type:int;default:5" json:"max_retries"`   // 最大重试次数。
	Status     MessageStatus `gorm:"column:status;type:tinyint;default:0;index" json:"status"`   // 状态。
}

// TableName 指定表名。
func (Message) TableName() string {
	return "sys_outbox_messages"
}

// Pusher 消息推送接口。
type Pusher interface {
	// Push 执行底层消息发送。
	Push(ctx context.Context, topic string, key string, payload []byte) error
}

// Manager 离群消息管理器，负责在业务事务中同步持久化消息记录。
type Manager struct {
	db     *gorm.DB     // 用于执行数据库操作的 GORM 实例。
	logger *slog.Logger // 日志记录器。
}

// NewManager 创建并返回一个新的 Outbox 管理器实例。
func NewManager(db *gorm.DB, logger *slog.Logger) *Manager {
	return &Manager{
		db:     db,
		logger: logger.With("module", "outbox"),
	}
}

// PublishInTx 在现有的数据库事务中持久化一条待发送的消息。
func (m *Manager) PublishInTx(ctx context.Context, tx *gorm.DB, topic, key string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal outbox payload: %w", err)
	}

	metadataMap := tracing.InjectContext(ctx)
	metadataJSON, err := json.Marshal(metadataMap)
	if err != nil {
		// 元数据注入失败不应中断业务事务，仅记录警告。
		m.logger.WarnContext(ctx, "failed to marshal tracing metadata", "error", err)
	}

	msg := &Message{
		Topic:     topic,
		Key:       key,
		Payload:   data,
		Metadata:  string(metadataJSON),
		Status:    StatusPending,
		NextRetry: time.Now(),
	}

	if createErr := tx.Create(msg).Error; createErr != nil {
		m.logger.ErrorContext(ctx, "failed to save outbox message", "topic", topic, "error", createErr)

		return createErr
	}

	return nil
}

// Processor 离群消息处理器，负责扫描 Outbox 表并异步投递消息。
type Processor struct {
	pusher        func(ctx context.Context, topic, key string, payload []byte) error
	mgr           *Manager
	stopChan      chan struct{}
	interval      time.Duration
	batchSize     int
	retentionDays int
}

// NewProcessor 创建处理器。
func NewProcessor(mgr *Manager, pusher func(ctx context.Context, topic, key string, payload []byte) error, batchSize int, interval time.Duration) *Processor {
	size := batchSize
	if size <= 0 {
		size = 100
	}

	inter := interval
	if inter <= 0 {
		inter = 5 * time.Second
	}

	return &Processor{
		mgr:           mgr,
		pusher:        pusher,
		batchSize:     size,
		interval:      inter,
		retentionDays: 7, // 默认保留 7 天。
		stopChan:      make(chan struct{}),
	}
}

// Start 启动后台任务。
func (p *Processor) Start() {
	p.mgr.logger.Info("outbox processor started")

	ticker := time.NewTicker(p.interval)
	cleanupTicker := time.NewTicker(24 * time.Hour) // 每天清理一次。

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

// Stop 停止处理器。
func (p *Processor) Stop() {
	close(p.stopChan)
}

// process 执行消息投递 (带集群安全并发控制)。
func (p *Processor) process() {
	var messages []*Message

	// 1. 抓取待处理消息 (短事务，仅用于锁定和标记)。
	// 核心架构决策：使用事务 + SKIP LOCKED 确保多实例不抢占同一批消息。
	err := p.mgr.db.Transaction(func(tx *gorm.DB) error {
		findErr := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND next_retry <= ? AND retry_count < max_retries", StatusPending, time.Now()).
			Limit(p.batchSize).
			Order("id ASC").
			Find(&messages).Error
		if findErr != nil {
			return findErr
		}

		if len(messages) == 0 {
			return nil
		}

		// 预先更新 NextRetry，防止本批次发送时间过长导致被其他实例重复抓取。
		ids := make([]uint, len(messages))
		for i, m := range messages {
			ids[i] = m.ID
		}
		// 锁定 1 分钟，给异步发送预留足够时间。
		return tx.Model(&Message{}).Where("id IN ?", ids).Update("next_retry", time.Now().Add(time.Minute)).Error
	})
	if err != nil {
		p.mgr.logger.Error("outbox batch fetch failed", "error", err)

		return
	}

	if len(messages) == 0 {
		return
	}

	// 2. 锁外并发投递。
	var wg sync.WaitGroup
	for _, msg := range messages {
		wg.Add(1)
		go func(m *Message) {
			defer wg.Done()
			p.send(m)
		}(msg)
	}
	wg.Wait()
}

// send 执行单条消息发送并更新状态。
func (p *Processor) send(msg *Message) {
	ctx := context.Background()
	if msg.Metadata != "" {
		var carrier map[string]string
		if err := json.Unmarshal([]byte(msg.Metadata), &carrier); err == nil {
			ctx = tracing.ExtractContext(ctx, carrier)
		}
	}

	ctx, span := tracing.Tracer().Start(ctx, "Outbox.Send")
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
		// 指数退避重试。
		// 安全：RetryCount 为正整数，位掩码限制最大重试次数。
		// G115 Fix: Masking
		retryCount := cast.IntToUint(msg.RetryCount & 0x3F)
		backoff := min(time.Duration(1<<retryCount)*time.Minute, 24*time.Hour)
		updates["next_retry"] = time.Now().Add(backoff)
		updates["last_error"] = err.Error()

		if msg.RetryCount+1 >= msg.MaxRetries {
			updates["status"] = StatusFailed
			p.mgr.logger.ErrorContext(ctx, "outbox message failed permanently", "id", msg.ID, "topic", msg.Topic)
		} else {
			p.mgr.logger.WarnContext(ctx, "outbox message send failed, retrying", "id", msg.ID, "error", err)
		}
	}

	// 更新消息状态 (独立事务)。
	if updateErr := p.mgr.db.Model(&msg).Updates(updates).Error; updateErr != nil {
		p.mgr.logger.ErrorContext(ctx, "failed to update outbox message status", "id", msg.ID, "error", updateErr)
	}
}

// cleanup 定期清理旧数据，防止表膨胀。
func (p *Processor) cleanup() {
	deadline := time.Now().AddDate(0, 0, -p.retentionDays)
	result := p.mgr.db.Where("status = ? AND updated_at < ?", StatusSent, deadline).Delete(&Message{})

	if result.Error != nil {
		p.mgr.logger.Error("failed to cleanup old outbox messages", "error", result.Error)
	} else if result.RowsAffected > 0 {
		p.mgr.logger.Info("outbox cleanup finished", "rows_deleted", result.RowsAffected)
	}
}
