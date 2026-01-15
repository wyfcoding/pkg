// Package lock 提供了基于 Redis 的分布式锁实现.
package lock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	// ErrLockFailed 获取锁失败.
	ErrLockFailed = errors.New("failed to acquire lock")
	// ErrUnlockFailed 释放锁失败.
	ErrUnlockFailed = errors.New("failed to release lock")

	unlockScript = redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`)

	renewScript = redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("expire", KEYS[1], ARGV[2])
		else
			return 0
		end
	`)
)

const (
	initialRetryInterval = 50 * time.Millisecond
	maxRetryInterval     = 500 * time.Millisecond
	watchdogIntervalDiv  = 3
	tokenBytes           = 16
)

// DistributedLock 定义分布式锁的增强型行为接口.
type DistributedLock interface {
	Lock(ctx context.Context, key string, ttl time.Duration) (token string, err error)
	TryLock(ctx context.Context, key string, ttl, waitTimeout time.Duration) (token string, err error)
	LockWithWatchdog(ctx context.Context, key string, ttl time.Duration) (token string, stop func(), err error)
	Unlock(ctx context.Context, key, token string) error
}

// RedisLock 是基于 Redis 实现的分布式锁结构.
type RedisLock struct {
	client redis.UniversalClient
}

// NewRedisLock 构造一个新的 Redis 分布式锁驱动.
func NewRedisLock(client redis.UniversalClient) *RedisLock {
	return &RedisLock{client: client}
}

// TryLock 阻塞式加锁实现.
func (l *RedisLock) TryLock(ctx context.Context, key string, ttl, waitTimeout time.Duration) (string, error) {
	token, err := l.generateToken()
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	timer := time.NewTimer(waitTimeout)
	defer timer.Stop()

	retryInterval := initialRetryInterval

	for {
		ok, err := l.client.SetNX(ctx, key, token, ttl).Result()
		if err == nil && ok {
			return token, nil
		}

		select {
		case <-timer.C:
			return "", ErrLockFailed
		case <-ctx.Done():
			return "", fmt.Errorf("context cancelled: %w", ctx.Err())
		case <-time.After(retryInterval):
			if retryInterval < maxRetryInterval {
				retryInterval *= 2
			}

			continue
		}
	}
}

// Lock 实现基础加锁逻辑 (一次性).
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

// LockWithWatchdog 具备自动续期能力的锁实现.
func (l *RedisLock) LockWithWatchdog(ctx context.Context, key string, ttl time.Duration) (token string, cancel func(), err error) {
	token, err = l.Lock(ctx, key, ttl)
	if err != nil {
		return "", nil, err
	}

	watchdogCtx, cancel := context.WithCancel(context.Background())

	go func() {
		ticker := time.NewTicker(ttl / watchdogIntervalDiv)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				res, err := renewScript.Run(watchdogCtx, l.client, []string{key}, token, int(ttl.Seconds())).Int()
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

	return token, cancel, nil
}

// Unlock 安全释放锁.
func (l *RedisLock) Unlock(ctx context.Context, key, token string) error {
	result, err := unlockScript.Run(ctx, l.client, []string{key}, token).Int()
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

// generateToken 生成加密安全的唯一令牌.
func (l *RedisLock) generateToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to read random bytes: %w", err)
	}

	return hex.EncodeToString(b) + strconv.FormatInt(time.Now().UnixNano(), 10), nil
}
