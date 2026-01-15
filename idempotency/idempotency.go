package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Status 定义请求的幂等状态.
type Status string

const (
	// StatusStarted 已开始处理.
	StatusStarted Status = "STARTED"
	// StatusFinished 已处理完成.
	StatusFinished Status = "FINISHED"
)

// ErrUnexpectedType 意外的响应类型.
var ErrUnexpectedType = errors.New("unexpected response type from redis")

// Response 用于存储已完成请求的响应快照.
type Response struct { // 幂等响应结构，已对齐。
	Header     map[string]string `json:"header"`
	Body       string            `json:"body"`
	StatusCode int               `json:"statusCode"`
}

// Manager 定义了幂等控制器的核心行为接口.
type Manager interface {
	TryStart(ctx context.Context, key string, ttl time.Duration) (bool, *Response, error)
	Finish(ctx context.Context, key string, resp *Response, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

var (
	// ErrInProgress 表示当前请求已存在且正在处理中.
	ErrInProgress = errors.New("request already in progress")

	startScript = redis.NewScript(`
		local val = redis.call("get", KEYS[1])
		if not val then
			redis.call("set", KEYS[1], ARGV[1], "EX", ARGV[2])
			return "OK"
		end
		return val
	`)
)

type redisManager struct { // Redis 幂等管理器内部结构，已对齐。
	client redis.UniversalClient
	prefix string
}

// NewRedisManager 初始化并返回一个基于 Redis 的幂等管理器.
func NewRedisManager(client redis.UniversalClient, prefix string) Manager {
	return &redisManager{
		client: client,
		prefix: prefix,
	}
}

func (m *redisManager) makeKey(key string) string {
	return m.prefix + ":" + key
}

func (m *redisManager) TryStart(ctx context.Context, key string, ttl time.Duration) (bool, *Response, error) {
	fullKey := m.makeKey(key)

	res, err := startScript.Run(ctx, m.client, []string{fullKey}, string(StatusStarted), int(ttl.Seconds())).Result()
	if err != nil {
		slog.Error("idempotency try_start error", "key", fullKey, "error", err)

		return false, nil, fmt.Errorf("failed to eval start script: %w", err)
	}

	if res == "OK" {
		slog.Debug("idempotency check passed (first time)", "key", fullKey)

		return true, nil, nil
	}

	if res == string(StatusStarted) {
		slog.Warn("idempotency check failed (in progress)", "key", fullKey)

		return false, nil, ErrInProgress
	}

	resStr, ok := res.(string)
	if !ok {
		return false, nil, fmt.Errorf("%w: %T", ErrUnexpectedType, res)
	}

	var savedResp Response
	if errUnmarshal := json.Unmarshal([]byte(resStr), &savedResp); errUnmarshal != nil {
		slog.Error("idempotency failed to unmarshal saved response", "key", fullKey, "error", errUnmarshal)

		return false, nil, fmt.Errorf("failed to unmarshal saved response: %w", errUnmarshal)
	}

	slog.Info("idempotency check hit (finished)", "key", fullKey)

	return false, &savedResp, nil
}

func (m *redisManager) Finish(ctx context.Context, key string, resp *Response, ttl time.Duration) error {
	fullKey := m.makeKey(key)
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	if errSet := m.client.Set(ctx, fullKey, string(data), ttl).Err(); errSet != nil {
		slog.Error("idempotency finish error", "key", fullKey, "error", errSet)

		return fmt.Errorf("failed to set finished status: %w", errSet)
	}

	slog.Debug("idempotency request finished and saved", "key", fullKey)

	return nil
}

func (m *redisManager) Delete(ctx context.Context, key string) error {
	if err := m.client.Del(ctx, m.makeKey(key)).Err(); err != nil {
		return fmt.Errorf("failed to delete idempotency key: %w", err)
	}

	return nil
}
