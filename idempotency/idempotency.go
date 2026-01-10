package idempotency

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Status 定义请求的幂等状.
type Status string

const (
	StatusStarted  Status = "STARTED"
	StatusFinished Status = "FINISHED"
)

// Response 用于存储已完成请求的响应快.
type Response struct {
	StatusCode int               `json:"status_code"`
	Header     map[string]string `json:"header"`
	Body       string            `json:"body"`
}

// Manager 定义了幂等控制器的核心行为接口。
type Manager interface {
	// TryStart 尝试开启幂等保护。
	// 返回值.
	// - isFirst: 是否为首次请.
	// - savedResponse: 如果是已完成的重复请求，返回之前存储的响应快.
	// - error: 如果请求正在处理中返回 ErrInProgress，或其他底层错.
	TryStart(ctx context.Context, key string, ttl time.Duration) (bool, *Response, error)

	// Finish 标记请求处理完成，并持久化响应快照。
	Finish(ctx context.Context, key string, resp *Response, ttl time.Duration) error

	// Delete 显式删除幂等记录，通常在业务逻辑明确失败且允许用户立即重试时调用。
	Delete(ctx context.Context, key string) error
}

// ErrInProgress 表示当前请求已存在且正在处理中，禁止重入。
var ErrInProgress = fmt.Errorf("request already in progress")

// redisManager 是幂等管理器的 Redis 实现。
type redisManager struct {
	client *redis.Client // Redis 客户端实.
	prefix string        // 键名全局前.
}

// NewRedisManager 初始化并返回一个基于 Redis 的幂等管理器。
func NewRedisManager(client *redis.Client, prefix string) Manager {
	return &redisManager{
		client: client,
		prefix: prefix,
	}
}

// makeKey 内部方法：构建包含前缀的完整 Redis 键名。
func (m *redisManager) makeKey(key string) string {
	return fmt.Sprintf("%s:%s", m.prefix, key)
}

func (m *redisManager) TryStart(ctx context.Context, key string, ttl time.Duration) (bool, *Response, error) {
	fullKey := m.makeKey(key)

	// Lua 脚本实现 Check-And-Set 原子语.
	script := `
		local val = redis.call("get", KEYS[1])
		if not val then
			redis.call("set", KEYS[1], ARGV[1], "EX", ARGV[2])
			return "OK"
		end
		return val
	`

	res, err := m.client.Eval(ctx, script, []string{fullKey}, string(StatusStarted), int(ttl.Seconds())).Result()
	if err != nil {
		slog.Error("idempotency try_start error", "key", fullKey, "error", err)
		return false, nil, err
	}

	if res == "OK" {
		slog.Debug("idempotency check passed (first time)", "key", fullKey)
		return true, nil, nil
	}

	if res == string(StatusStarted) {
		slog.Warn("idempotency check failed (in progress)", "key", fullKey)
		return false, nil, ErrInProgress
	}

	var savedResp Response
	if err := json.Unmarshal([]byte(res.(string)), &savedResp); err != nil {
		slog.Error("idempotency failed to unmarshal saved response", "key", fullKey, "error", err)
		return false, nil, fmt.Errorf("failed to unmarshal saved response: %w", err)
	}

	slog.Info("idempotency check hit (finished)", "key", fullKey)
	return false, &savedResp, nil
}

func (m *redisManager) Finish(ctx context.Context, key string, resp *Response, ttl time.Duration) error {
	fullKey := m.makeKey(key)
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	err = m.client.Set(ctx, fullKey, string(data), ttl).Err()
	if err != nil {
		slog.Error("idempotency finish error", "key", fullKey, "error", err)
		return err
	}
	slog.Debug("idempotency request finished and saved", "key", fullKey)
	return nil
}

func (m *redisManager) Delete(ctx context.Context, key string) error {
	return m.client.Del(ctx, m.makeKey(key)).Err()
}
