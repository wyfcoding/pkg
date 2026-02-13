// Package bloom 提供了布隆过滤器的实现。
// 增强：实现了基于 Redis Bitset 的分布式布隆过滤器，防止缓存击穿与海量数据判重。
package bloom

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/redis/go-redis/v9"
)

// RedisBloom 分布式布隆过滤器。
type RedisBloom struct {
	client *redis.Client
	key    string
	size   uint64 // 位数组大小
	hashes uint   // 哈希函数个数
}

// NewRedisBloom 创建一个新的分布式布隆过滤器。
func NewRedisBloom(client *redis.Client, key string, size uint64, hashes uint) *RedisBloom {
	return &RedisBloom{
		client: client,
		key:    key,
		size:   size,
		hashes: hashes,
	}
}

// Add 向过滤器中增加一个元素。
func (b *RedisBloom) Add(ctx context.Context, item string) error {
	offsets := b.getOffsets(item)
	pipe := b.client.Pipeline()
	for _, offset := range offsets {
		pipe.SetBit(ctx, b.key, int64(offset), 1)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// Exist 检查元素是否存在于过滤器中（可能存在误判）。
func (b *RedisBloom) Exist(ctx context.Context, item string) (bool, error) {
	offsets := b.getOffsets(item)
	pipe := b.client.Pipeline()
	for _, offset := range offsets {
		pipe.GetBit(ctx, b.key, int64(offset))
	}
	cmds, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return false, err
	}

	for _, cmd := range cmds {
		bit, _ := cmd.(*redis.IntCmd).Result()
		if bit == 0 {
			return false, nil
		}
	}
	return true, nil
}

func (b *RedisBloom) getOffsets(item string) []uint64 {
	offsets := make([]uint64, b.hashes)
	h := fnv.New64a()
	for i := uint(0); i < b.hashes; i++ {
		h.Reset()
		h.Write(fmt.Appendf(nil, "%s-%d", item, i))
		offsets[i] = h.Sum64() % b.size
	}
	return offsets
}
