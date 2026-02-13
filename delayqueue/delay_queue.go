// 变更说明：延迟队列组件，支持基于 Redis Sorted Set 和时间轮的延迟任务处理
package delayqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// DelayMessage 延迟消息
type DelayMessage struct {
	ID         string `json:"id"`
	Topic      string `json:"topic"`
	Payload    any    `json:"payload"`
	DelayAt    int64  `json:"delay_at"`   // 执行时间戳（毫秒）
	CreatedAt  int64  `json:"created_at"` // 创建时间戳
	RetryCount int    `json:"retry_count"`
	MaxRetry   int    `json:"max_retry"`
}

// DelayQueue 延迟队列接口
type DelayQueue interface {
	// Push 添加延迟消息
	Push(ctx context.Context, topic string, payload any, delay time.Duration) (string, error)
	// PushWithID 添加带ID的延迟消息
	PushWithID(ctx context.Context, id, topic string, payload any, delay time.Duration) error
	// Pop 弹出到期消息（阻塞）
	Pop(ctx context.Context, topic string, timeout time.Duration) (*DelayMessage, error)
	// BatchPop 批量弹出到期消息
	BatchPop(ctx context.Context, topic string, count int) ([]*DelayMessage, error)
	// Remove 移除消息
	Remove(ctx context.Context, topic, id string) error
	// Size 获取队列大小
	Size(ctx context.Context, topic string) (int64, error)
}

// RedisDelayQueue 基于 Redis Sorted Set 的延迟队列
type RedisDelayQueue struct {
	client    redis.UniversalClient
	keyPrefix string
}

// NewRedisDelayQueue 创建 Redis 延迟队列
func NewRedisDelayQueue(client redis.UniversalClient, keyPrefix string) *RedisDelayQueue {
	if keyPrefix == "" {
		keyPrefix = "delay_queue"
	}
	return &RedisDelayQueue{
		client:    client,
		keyPrefix: keyPrefix,
	}
}

// Push 添加延迟消息
func (q *RedisDelayQueue) Push(ctx context.Context, topic string, payload any, delay time.Duration) (string, error) {
	id := fmt.Sprintf("DM%d", time.Now().UnixNano())
	err := q.PushWithID(ctx, id, topic, payload, delay)
	return id, err
}

// PushWithID 添加带ID的延迟消息
func (q *RedisDelayQueue) PushWithID(ctx context.Context, id, topic string, payload any, delay time.Duration) error {
	now := time.Now()
	delayAt := now.Add(delay).UnixMilli()

	msg := &DelayMessage{
		ID:        id,
		Topic:     topic,
		Payload:   payload,
		DelayAt:   delayAt,
		CreatedAt: now.UnixMilli(),
		MaxRetry:  3,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	key := q.getQueueKey(topic)
	return q.client.ZAdd(ctx, key, redis.Z{
		Score:  float64(delayAt),
		Member: string(data),
	}).Err()
}

// Pop 弹出到期消息
func (q *RedisDelayQueue) Pop(ctx context.Context, topic string, timeout time.Duration) (*DelayMessage, error) {
	messages, err := q.BatchPop(ctx, topic, 1)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, nil
	}
	return messages[0], nil
}

// BatchPop 批量弹出到期消息
func (q *RedisDelayQueue) BatchPop(ctx context.Context, topic string, count int) ([]*DelayMessage, error) {
	key := q.getQueueKey(topic)
	now := time.Now().UnixMilli()

	// 使用 Lua 脚本原子性地获取并删除到期消息
	script := `
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local count = tonumber(ARGV[2])
		
		local messages = redis.call('ZRANGEBYSCORE', key, 0, now, 'LIMIT', 0, count)
		if #messages > 0 then
			for i, msg in ipairs(messages) do
				redis.call('ZREM', key, msg)
			end
		end
		return messages
	`

	result, err := q.client.Eval(ctx, script, []string{key}, now, count).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to pop messages: %w", err)
	}

	messages := make([]*DelayMessage, 0)
	if items, ok := result.([]any); ok {
		for _, item := range items {
			if data, ok := item.(string); ok {
				var msg DelayMessage
				if err := json.Unmarshal([]byte(data), &msg); err == nil {
					messages = append(messages, &msg)
				}
			}
		}
	}

	return messages, nil
}

// Remove 移除消息
func (q *RedisDelayQueue) Remove(ctx context.Context, topic, id string) error {
	key := q.getQueueKey(topic)

	// 获取所有消息并查找指定ID
	messages, err := q.client.ZRange(ctx, key, 0, -1).Result()
	if err != nil {
		return err
	}

	for _, data := range messages {
		var msg DelayMessage
		if err := json.Unmarshal([]byte(data), &msg); err == nil {
			if msg.ID == id {
				return q.client.ZRem(ctx, key, data).Err()
			}
		}
	}

	return nil
}

// Size 获取队列大小
func (q *RedisDelayQueue) Size(ctx context.Context, topic string) (int64, error) {
	key := q.getQueueKey(topic)
	return q.client.ZCard(ctx, key).Result()
}

// getQueueKey 获取队列键
func (q *RedisDelayQueue) getQueueKey(topic string) string {
	return fmt.Sprintf("%s:%s", q.keyPrefix, topic)
}

// Consumer 消费者接口
type Consumer interface {
	// Consume 消费消息
	Consume(ctx context.Context, msg *DelayMessage) error
	// Topic 返回消费的主题
	Topic() string
}

// DelayQueueWorker 延迟队列工作器
type DelayQueueWorker struct {
	queue     DelayQueue
	consumers map[string]Consumer
	interval  time.Duration
	running   atomic.Bool
	wg        sync.WaitGroup
	logger    *slog.Logger
}

// NewDelayQueueWorker 创建延迟队列工作器
func NewDelayQueueWorker(queue DelayQueue, interval time.Duration, logger *slog.Logger) *DelayQueueWorker {
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	return &DelayQueueWorker{
		queue:     queue,
		consumers: make(map[string]Consumer),
		interval:  interval,
		logger:    logger,
	}
}

// RegisterConsumer 注册消费者
func (w *DelayQueueWorker) RegisterConsumer(consumer Consumer) {
	w.consumers[consumer.Topic()] = consumer
}

// Start 启动工作器
func (w *DelayQueueWorker) Start(ctx context.Context) {
	if w.running.Swap(true) {
		return
	}

	w.wg.Add(1)
	go w.run(ctx)
}

// Stop 停止工作器
func (w *DelayQueueWorker) Stop() {
	w.running.Store(false)
	w.wg.Wait()
}

// run 运行循环
func (w *DelayQueueWorker) run(ctx context.Context) {
	defer w.wg.Done()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !w.running.Load() {
				return
			}
			w.processMessages(ctx)
		}
	}
}

// processMessages 处理消息
func (w *DelayQueueWorker) processMessages(ctx context.Context) {
	for topic, consumer := range w.consumers {
		messages, err := w.queue.BatchPop(ctx, topic, 10)
		if err != nil {
			w.logger.ErrorContext(ctx, "failed to pop messages", "topic", topic, "error", err)
			continue
		}

		for _, msg := range messages {
			if err := w.processMessage(ctx, consumer, msg); err != nil {
				w.logger.ErrorContext(ctx, "failed to process message", "id", msg.ID, "error", err)
			}
		}
	}
}

// processMessage 处理单条消息
func (w *DelayQueueWorker) processMessage(ctx context.Context, consumer Consumer, msg *DelayMessage) error {
	if err := consumer.Consume(ctx, msg); err != nil {
		if msg.RetryCount < msg.MaxRetry {
			msg.RetryCount++
			delay := time.Duration(msg.RetryCount) * time.Minute
			return w.queue.PushWithID(ctx, msg.ID, msg.Topic, msg.Payload, delay)
		}
		return fmt.Errorf("max retry exceeded: %w", err)
	}
	return nil
}

// TimeWheelDelayQueue 时间轮延迟队列（本地内存实现）
type TimeWheelDelayQueue struct {
	tick      time.Duration
	wheelSize int
	buckets   []*bucket
	current   int64
	running   atomic.Bool
	wg        sync.WaitGroup
	logger    *slog.Logger
	mu        sync.RWMutex
}

type bucket struct {
	mu    sync.Mutex
	items map[string]*DelayMessage
}

// NewTimeWheelDelayQueue 创建时间轮延迟队列
func NewTimeWheelDelayQueue(tick time.Duration, wheelSize int, logger *slog.Logger) *TimeWheelDelayQueue {
	if tick <= 0 {
		tick = 100 * time.Millisecond
	}
	if wheelSize <= 0 {
		wheelSize = 3600 // 默认支持1小时延迟
	}

	buckets := make([]*bucket, wheelSize)
	for i := range buckets {
		buckets[i] = &bucket{items: make(map[string]*DelayMessage)}
	}

	return &TimeWheelDelayQueue{
		tick:      tick,
		wheelSize: wheelSize,
		buckets:   buckets,
		current:   time.Now().UnixMilli(),
		logger:    logger,
	}
}

// Push 添加延迟消息
func (q *TimeWheelDelayQueue) Push(ctx context.Context, topic string, payload any, delay time.Duration) (string, error) {
	id := fmt.Sprintf("TW%d", time.Now().UnixNano())
	err := q.PushWithID(ctx, id, topic, payload, delay)
	return id, err
}

// PushWithID 添加带ID的延迟消息
func (q *TimeWheelDelayQueue) PushWithID(ctx context.Context, id, topic string, payload any, delay time.Duration) error {
	now := time.Now()
	delayAt := now.Add(delay).UnixMilli()

	msg := &DelayMessage{
		ID:        id,
		Topic:     topic,
		Payload:   payload,
		DelayAt:   delayAt,
		CreatedAt: now.UnixMilli(),
		MaxRetry:  3,
	}

	q.mu.RLock()
	defer q.mu.RUnlock()

	index := q.getIndex(delayAt)
	q.buckets[index].mu.Lock()
	q.buckets[index].items[id] = msg
	q.buckets[index].mu.Unlock()

	return nil
}

// Pop 弹出到期消息
func (q *TimeWheelDelayQueue) Pop(ctx context.Context, topic string, timeout time.Duration) (*DelayMessage, error) {
	messages, err := q.BatchPop(ctx, topic, 1)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, nil
	}
	return messages[0], nil
}

// BatchPop 批量弹出到期消息
func (q *TimeWheelDelayQueue) BatchPop(ctx context.Context, topic string, count int) ([]*DelayMessage, error) {
	now := time.Now().UnixMilli()
	index := q.getIndex(now)

	q.buckets[index].mu.Lock()
	defer q.buckets[index].mu.Unlock()

	messages := make([]*DelayMessage, 0, count)
	for id, msg := range q.buckets[index].items {
		if msg.DelayAt <= now && msg.Topic == topic {
			messages = append(messages, msg)
			delete(q.buckets[index].items, id)
			if len(messages) >= count {
				break
			}
		}
	}

	return messages, nil
}

// Remove 移除消息
func (q *TimeWheelDelayQueue) Remove(ctx context.Context, topic, id string) error {
	for _, b := range q.buckets {
		b.mu.Lock()
		if msg, ok := b.items[id]; ok && msg.Topic == topic {
			delete(b.items, id)
			b.mu.Unlock()
			return nil
		}
		b.mu.Unlock()
	}
	return nil
}

// Size 获取队列大小
func (q *TimeWheelDelayQueue) Size(ctx context.Context, topic string) (int64, error) {
	var size int64
	for _, b := range q.buckets {
		b.mu.Lock()
		for _, msg := range b.items {
			if msg.Topic == topic {
				size++
			}
		}
		b.mu.Unlock()
	}
	return size, nil
}

// getIndex 获取时间轮索引
func (q *TimeWheelDelayQueue) getIndex(timestamp int64) int {
	tickMs := q.tick.Milliseconds()
	return int((timestamp / tickMs) % int64(q.wheelSize))
}

// Start 启动时间轮
func (q *TimeWheelDelayQueue) Start(ctx context.Context) {
	if q.running.Swap(true) {
		return
	}

	q.wg.Add(1)
	go q.run(ctx)
}

// Stop 停止时间轮
func (q *TimeWheelDelayQueue) Stop() {
	q.running.Store(false)
	q.wg.Wait()
}

// run 运行时间轮
func (q *TimeWheelDelayQueue) run(ctx context.Context) {
	defer q.wg.Done()

	ticker := time.NewTicker(q.tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if !q.running.Load() {
				return
			}
			q.current = now.UnixMilli()
		}
	}
}
