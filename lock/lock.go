package lock

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9" // 导入Redis客户端库。
)

// 定义分布式锁操作可能返回的错误。
var (
	ErrLockFailed   = errors.New("failed to acquire lock") // 获取锁失败。
	ErrUnlockFailed = errors.New("failed to release lock") // 释放锁失败。
)

// DistributedLock 接口定义了分布式锁的通用行为。
// 任何实现了此接口的类型都可以用作分布式锁。
type DistributedLock interface {
	// Lock 尝试获取一个指定键的分布式锁。
	// ctx: 上下文，用于控制操作的生命周期。
	// key: 锁的唯一标识符。
	// ttl: 锁的过期时间，防止死锁。
	// 返回值：一个唯一的令牌（token），用于释放锁；如果failed to fetch则返回错误。
	Lock(ctx context.Context, key string, ttl time.Duration) (string, error)
	// Unlock 尝试释放一个指定键的分布式锁。
	// ctx: 上下文。
	// key: 锁的唯一标识符。
	// token: 获取锁时返回的令牌，用于确保只有锁的持有者才能释放锁。
	// 返回值：如果释放失败则返回错误。
	Unlock(ctx context.Context, key string, token string) error
}

// RedisLock 结构体实现了基于 Redis 的分布式锁。
// 它利用Redis的原子操作和SETNX命令来确保锁的互斥性和安全性。
type RedisLock struct {
	client *redis.Client // Redis客户端实例。
}

// NewRedisLock 创建并返回一个新的 RedisLock 实例。
// client: 已连接的Redis客户端实例。
func NewRedisLock(client *redis.Client) *RedisLock {
	return &RedisLock{client: client}
}

// Lock 尝试获取一个分布式锁。
// 使用 `SETNX` (SET if Not eXists) 命令来确保原子性。
// 如果键不存在，则设置键值对并返回成功；否则不执行任何操作并返回失败。
func (l *RedisLock) Lock(ctx context.Context, key string, ttl time.Duration) (string, error) {
	token := generateToken() // 生成一个唯一的令牌，用于防止误删。
	// SETNX 命令原子地执行设置和判断操作。
	// key: 锁的键名。
	// value: 锁的值，这里使用唯一的token。
	// expiration: 锁的过期时间，防止死锁。
	ok, err := l.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return "", fmt.Errorf("redis setnx failed: %w", err)
	}
	if !ok {
		return "", ErrLockFailed // 如果SetNX返回false，表示锁已被其他客户端持有。
	}
	return token, nil // 获取锁成功，返回生成的token。
}

// Unlock 尝试释放一个分布式锁。
// 为了确保只有锁的持有者才能释放锁，使用Lua脚本实现原子性的“GET然后DEL”操作。
func (l *RedisLock) Unlock(ctx context.Context, key string, token string) error {
	// Lua脚本确保了在获取锁的值和删除锁这两个操作之间不会有其他客户端修改锁的状态。
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`
	// Eval 命令执行Lua脚本。
	// KEYS[1]: 锁的键名。
	// ARGV[1]: 锁的值（即token）。
	result, err := l.client.Eval(ctx, script, []string{key}, token).Result()
	if err != nil {
		return fmt.Errorf("redis eval unlock script failed: %w", err)
	}
	// Lua脚本返回0表示锁的值不匹配（不是持有者），或者锁不存在。
	if result.(int64) == 0 {
		return ErrUnlockFailed
	}
	return nil // 释放锁成功。
}

// generateToken 生成一个唯一的字符串令牌。
// 它可以用于作为锁的值，以区分不同的锁持有者，防止误删。
func generateToken() string {
	// 使用当前时间戳（纳秒级）的格式化字符串作为令牌，保证唯一性。
	return time.Now().Format("20060102150405.000000")
}
