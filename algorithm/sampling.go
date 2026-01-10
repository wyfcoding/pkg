package algorithm

import (
	"math/rand/v2"
	"time"
)

// ReservoirSampler 蓄水池采样.
type ReservoirSampler[T any] struct {
	random  *rand.Rand
	samples []T
	count   int
	k       int
}

// NewReservoirSampler 创建一个新的 ReservoirSampler 实例.
func NewReservoirSampler[T any](k int) *ReservoirSampler[T] {
	//nolint:gosec // 采样算法使用非加密安全随机数以保证性能.
	r := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))

	return &ReservoirSampler[T]{
		k:       k,
		samples: make([]T, 0, k),
		random:  r,
		count:   0,
	}
}

// Observe 处理一个新到达的元素.
func (s *ReservoirSampler[T]) Observe(item T) {
	s.count++

	if len(s.samples) < s.k {
		s.samples = append(s.samples, item)
	} else {
		j := s.random.IntN(s.count)
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
