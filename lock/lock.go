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
	// LockWithWatchdog 加锁并启动自动续期协程
	// 它返回一个 stop 函数，业务执行完后必须调用 stop 停止看门狗
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

// Lock 实现基础加锁逻辑
func (l *RedisLock) Lock(ctx context.Context, key string, ttl time.Duration) (string, error) {
	token := l.generateToken()
	ok, err := l.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return "", fmt.Errorf("redis setnx failed: %w", err)
	}
	if !ok {
		return "", ErrLockFailed
	}
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
		return fmt.Errorf("redis eval unlock failed: %w", err)
	}
	if result == 0 {
		return ErrUnlockFailed
	}
	return nil
}

// generateToken 生成加密安全的唯一令牌
func (l *RedisLock) generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b) + fmt.Sprintf("%d", time.Now().UnixNano())
}