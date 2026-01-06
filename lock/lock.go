// Package lock 提供了基于 Redis 的分布式锁实现，支持阻塞等待、看门狗自动续期及安全性释放。
package lock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrLockFailed   = errors.New("failed to acquire lock") // 获取锁失败（已被占用或网络错误）
	ErrUnlockFailed = errors.New("failed to release lock") // 释放锁失败（非所有者或已过期）
)

// DistributedLock 定义分布式锁的增强型行为接口。
type DistributedLock interface {
	// Lock 尝试非阻塞式加锁。
	Lock(ctx context.Context, key string, ttl time.Duration) (string, error)
	// TryLock 尝试阻塞式加锁，在 waitTimeout 期限内不断重试。
	TryLock(ctx context.Context, key string, ttl time.Duration, waitTimeout time.Duration) (string, error)
	// LockWithWatchdog 加锁并启动看门狗协程，在持有锁期间自动续期。
	LockWithWatchdog(ctx context.Context, key string, ttl time.Duration) (string, func(), error)
	// Unlock 安全释放锁，内部通过 Token 校验确保“谁加锁谁释放”。
	Unlock(ctx context.Context, key string, token string) error
}

// RedisLock 是基于 Redis 实现的分布式锁结构。
type RedisLock struct {
	client *redis.Client // Redis 客户端实例
}

// NewRedisLock 构造一个新的 Redis 分布式锁驱动。
func NewRedisLock(client *redis.Client) *RedisLock {
	return &RedisLock{client: client}
}

// TryLock 阻塞式加锁实现，采用指数退避重试策略。
func (l *RedisLock) TryLock(ctx context.Context, key string, ttl time.Duration, waitTimeout time.Duration) (string, error) {
	token, err := l.generateToken()
	if err != nil {
		return "", err
	}

	timer := time.NewTimer(waitTimeout)
	defer timer.Stop()

	retryInterval := 50 * time.Millisecond

	for {
		ok, err := l.client.SetNX(ctx, key, token, ttl).Result()
		if err == nil && ok {
			return token, nil
		}

		select {
		case <-timer.C:
			return "", ErrLockFailed
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(retryInterval):
			if retryInterval < 500*time.Millisecond {
				retryInterval *= 2
			}
			continue
		}
	}
}

// Lock 实现基础加锁逻辑 (一次性)。
func (l *RedisLock) Lock(ctx context.Context, key string, ttl time.Duration) (string, error) {
	token, err := l.generateToken()
	if err != nil {
		return "", fmt.Errorf("generate token failed: %w", err)
	}
	ok, err := l.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		slog.Error("redis_lock error", "key", key, "error", err)
		return "", fmt.Errorf("redis setnx failed: %w", err)
	}
	if !ok {
		slog.Warn("redis_lock failed (already locked)", "key", key)
		return "", ErrLockFailed
	}
	slog.Debug("redis_lock acquired", "key", key)
	return token, nil
}

// LockWithWatchdog 具备自动续期能力的锁实现。
func (l *RedisLock) LockWithWatchdog(ctx context.Context, key string, ttl time.Duration) (string, func(), error) {
	token, err := l.Lock(ctx, key, ttl)
	if err != nil {
		return "", nil, err
	}

	watchdogCtx, cancel := context.WithCancel(context.Background())

	go func() {
		ticker := time.NewTicker(ttl / 3)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				script := `
					if redis.call("get", KEYS[1]) == ARGV[1] then
						return redis.call("expire", KEYS[1], ARGV[2])
					else
						return 0
					end
				`
				res, err := l.client.Eval(watchdogCtx, script, []string{key}, token, int(ttl.Seconds())).Int()
				if err != nil || res == 0 {
					slog.Warn("watchdog failed to renew lock", "key", key, "error", err)
					return
				}
				slog.Debug("watchdog renewed lock", "key", key)
			case <-watchdogCtx.Done():
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	stop := func() {
		cancel()
	}

	return token, stop, nil
}

// Unlock 安全释放锁，使用 Lua 脚本保证“校验 Token”与“删除 Key”的原子性。
func (l *RedisLock) Unlock(ctx context.Context, key string, token string) error {
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`
	result, err := l.client.Eval(ctx, script, []string{key}, token).Int()
	if err != nil {
		slog.Error("redis_unlock error", "key", key, "error", err)
		return fmt.Errorf("redis eval unlock failed: %w", err)
	}
	if result == 0 {
		slog.Warn("redis_unlock failed (not owner or key expired)", "key", key)
		return ErrUnlockFailed
	}
	slog.Debug("redis_lock released", "key", key)
	return nil
}

// generateToken 生成加密安全的唯一令牌。
func (l *RedisLock) generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b) + fmt.Sprintf("%d", time.Now().UnixNano()), nil
}
