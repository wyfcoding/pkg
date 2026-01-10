package sim

import (
	"crypto/rand"
	"encoding/binary"
)

// ReservoirSampler 蓄水池采样.
type ReservoirSampler[T any] struct {
	samples []T
	count   int
	k       int
}

// NewReservoirSampler 创建一个新的 ReservoirSampler 实例.
func NewReservoirSampler[T any](k int) *ReservoirSampler[T] {
	return &ReservoirSampler[T]{
		k:       k,
		samples: make([]T, 0, k),
		count:   0,
	}
}

// Observe 处理一个新到达的元素.
func (s *ReservoirSampler[T]) Observe(item T) {
	s.count++

	if len(s.samples) < s.k {
		s.samples = append(s.samples, item)
	} else {
		var b [8]byte
		_, _ = rand.Read(b[:])
		// 安全：count 为正数，用于随机选择索引。
		j := int(binary.LittleEndian.Uint64(b[:]) % uint64(s.count)) //nolint:gosec // count > 0 已保证。
		if j < s.k {
			s.samples[j] = item
		}
	}
}

// GetSamples 获取当前池中的所有样本.
func (s *ReservoirSampler[T]) GetSamples() []T {
	return s.samples
}

// Reset 重置采样器.
func (s *ReservoirSampler[T]) Reset() {
	s.count = 0
	s.samples = s.samples[:0]
}
