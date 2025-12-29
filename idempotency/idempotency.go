package idempotency

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Status 定义请求的幂等状态
type Status string

const (
	StatusStarted  Status = "STARTED"
	StatusFinished Status = "FINISHED"
)

// Response 用于存储已完成请求的响应快照
type Response struct {
	StatusCode int               `json:"status_code"`
	Header     map[string]string `json:"header"`
	Body       string            `json:"body"`
}

// Manager 幂等管理器接口
type Manager interface {
	// TryStart 尝试开始一个幂等请求。
	// 返回 (isFirst, savedResponse, error)
	// 如果是第一次请求，返回 (true, nil, nil)
	// 如果请求已完成，返回 (false, response, nil)
	// 如果请求正在处理中，返回 (false, nil, ErrInProgress)
	TryStart(ctx context.Context, key string, ttl time.Duration) (bool, *Response, error)

	// Finish 完成幂等请求，存储响应结果
	Finish(ctx context.Context, key string, resp *Response, ttl time.Duration) error

	// Delete 删除幂等记录，通常用于处理失败后允许重试
	Delete(ctx context.Context, key string) error
}

var ErrInProgress = fmt.Errorf("request already in progress")

type redisManager struct {
	client *redis.Client
	prefix string
}

func NewRedisManager(client *redis.Client, prefix string) Manager {
	return &redisManager{
		client: client,
		prefix: prefix,
	}
}

func (m *redisManager) makeKey(key string) string {
	return fmt.Sprintf("%s:%s", m.prefix, key)
}

func (m *redisManager) TryStart(ctx context.Context, key string, ttl time.Duration) (bool, *Response, error) {
	fullKey := m.makeKey(key)

	// Lua 脚本：原子性检查和设置
	// 1. 如果 key 不存在，设置状态为 STARTED 并设置过期时间 (NX)
	// 2. 如果存在且状态为 STARTED，返回 "IN_PROGRESS"
	// 3. 如果存在且状态为 FINISHED，返回缓存的 Response 数据
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
		return false, nil, err
	}

	if res == "OK" {
		return true, nil, nil
	}

	if res == string(StatusStarted) {
		return false, nil, ErrInProgress
	}

	// 如果是已完成的状态，res 存储的是 JSON 格式的 Response
	var savedResp Response
	if err := json.Unmarshal([]byte(res.(string)), &savedResp); err != nil {
		return false, nil, fmt.Errorf("failed to unmarshal saved response: %w", err)
	}

	return false, &savedResp, nil
}

func (m *redisManager) Finish(ctx context.Context, key string, resp *Response, ttl time.Duration) error {
	fullKey := m.makeKey(key)
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	// 更新状态为结果 JSON，并延长过期时间
	return m.client.Set(ctx, fullKey, string(data), ttl).Err()
}

func (m *redisManager) Delete(ctx context.Context, key string) error {
	return m.client.Del(ctx, m.makeKey(key)).Err()
}
