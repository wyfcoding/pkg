package sim

import (
	"crypto/rand"
	"encoding/binary"

	"github.com/wyfcoding/pkg/cast"
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
		// G115 Fix: use cast.IntToUint64 to bypass overflow warning.
		j := cast.Uint64ToIntSafe(binary.LittleEndian.Uint64(b[:]) % cast.IntToUint64(s.count))
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
