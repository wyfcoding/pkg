// Package lock 分布式锁增强
package lock

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
)

type RedLock struct {
	client *redis.Client
}

func (l *RedLock) Acquire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return l.client.SetNX(ctx, "lock:"+key, "1", ttl).Result()
}

func (l *RedLock) Release(ctx context.Context, key string) error {
	return l.client.Del(ctx, "lock:"+key).Err()
}

// ReentrantLock 可重入锁 (基于 Lua 脚本实现计数器)
const reentrantLua = `
if (redis.call('exists', KEYS[1]) == 0) or (redis.call('hexists', KEYS[1], ARGV[1]) == 1) then
    redis.call('hincrby', KEYS[1], ARGV[1], 1)
    redis.call('pexpire', KEYS[1], ARGV[2])
    return 1
end
return 0
`
