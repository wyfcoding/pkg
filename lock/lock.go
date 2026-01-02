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
	ErrLockFailed   = errors.New("failed to acquire lock")
	ErrUnlockFailed = errors.New("failed to release lock")
)

// DistributedLock 定义分布式锁的增强接口
type DistributedLock interface {
	// Lock 尝试加锁 (一次性)
	Lock(ctx context.Context, key string, ttl time.Duration) (string, error)
	// TryLock 阻塞式加锁，在 waitTimeout 内不断重试
	TryLock(ctx context.Context, key string, ttl time.Duration, waitTimeout time.Duration) (string, error)
	// LockWithWatchdog 加锁并启动自动续期协程
	LockWithWatchdog(ctx context.Context, key string, ttl time.Duration) (string, func(), error)
	// Unlock 安全释放锁 (校验所有权)
	Unlock(ctx context.Context, key string, token string) error
}

type RedisLock struct {
	client *redis.Client
}

func NewRedisLock(client *redis.Client) *RedisLock {
	return &RedisLock{client: client}
}

// TryLock 实现带重试机制的阻塞加锁
func (l *RedisLock) TryLock(ctx context.Context, key string, ttl time.Duration, waitTimeout time.Duration) (string, error) {
	token, err := l.generateToken()
	if err != nil {
		return "", err
	}

	timer := time.NewTimer(waitTimeout)
	defer timer.Stop()

	// 初始重试间隔
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
			// 指数退避，防止大量请求冲击 Redis
			if retryInterval < 500*time.Millisecond {
				retryInterval *= 2
			}
			continue
		}
	}
}

// Lock 实现基础加锁逻辑 (一次性)
func (l *RedisLock) Lock(ctx context.Context, key string, ttl time.Duration) (string, error) {
	token, err := l.generateToken()
	if err != nil {
		return "", fmt.Errorf("generate token failed: %w", err)
	}
	ok, err := l.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		slog.Error("Redis Lock error", "key", key, "error", err)
		return "", fmt.Errorf("redis setnx failed: %w", err)
	}
	if !ok {
		slog.Warn("Redis Lock failed (already locked)", "key", key)
		return "", ErrLockFailed
	}
	slog.Debug("Redis Lock acquired", "key", key)
	return token, nil
}

// LockWithWatchdog 具备自动续期能力的锁
func (l *RedisLock) LockWithWatchdog(ctx context.Context, key string, ttl time.Duration) (string, func(), error) {
	token, err := l.Lock(ctx, key, ttl)
	if err != nil {
		return "", nil, err
	}

	// 启动看门狗协程
	// 我们创建一个内部的 cancel context 来控制看门狗的生命周期
	watchdogCtx, cancel := context.WithCancel(context.Background())

	go func() {
		ticker := time.NewTicker(ttl / 3) // 每 1/3 TTL 续期一次
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// 续期 Lua 脚本：只有 key 的值等于 token 时才更新 TTL
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
					return // 续期失败或已失去所有权，退出协程
				}
				slog.Debug("watchdog renewed lock", "key", key)
			case <-watchdogCtx.Done():
				return // 外部主动调用了 stop，结束续期
			case <-ctx.Done():
				return // 业务 Context 结束，结束续期
			}
		}
	}()

	// 返回清理函数
	stop := func() {
		cancel()
		// 注意：这里不自动解锁，由业务代码显式调用 Unlock，
		// 这样可以处理业务报错时手动释放或依赖 TTL 自动释放的精细控制。
	}

	return token, stop, nil
}

// Unlock 实现安全解锁 (Lua 脚本保证原子性)
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
		slog.Error("Redis Unlock error", "key", key, "error", err)
		return fmt.Errorf("redis eval unlock failed: %w", err)
	}
	if result == 0 {
		slog.Warn("Redis Unlock failed (not owner or key expired)", "key", key)
		return ErrUnlockFailed
	}
	slog.Debug("Redis Lock released", "key", key)
	return nil
}

// generateToken 生成加密安全的唯一令牌
func (l *RedisLock) generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b) + fmt.Sprintf("%d", time.Now().UnixNano()), nil
}
