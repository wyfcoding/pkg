package algorithm

import (
	"math/rand"
	"time"
)

// ReservoirSampler 蓄水池采样器
// 用于从海量流数据中进行公平抽样
type ReservoirSampler[T any] struct {
	k       int // 期望采样的样本数量
	count   int // 已经流过的元素总数
	samples []T // 存储当前的样本
	random  *rand.Rand
}

func NewReservoirSampler[T any](k int) *ReservoirSampler[T] {
	return &ReservoirSampler[T]{
		k:       k,
		samples: make([]T, 0, k),
		random:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Observe 处理一个新到达的元素
func (s *ReservoirSampler[T]) Observe(item T) {
	s.count++

	if len(s.samples) < s.k {
		// 1. 如果池子还没满，直接放入
		s.samples = append(s.samples, item)
	} else {
		// 2. 如果池子满了，以 k/n 的概率替换掉池子里的一个旧元素
		j := s.random.Intn(s.count)
		if j < s.k {
			s.samples[j] = item
		}
	}
}

// GetSamples 获取当前池中的所有样本
func (s *ReservoirSampler[T]) GetSamples() []T {
	return s.samples
}

// Reset 重置采样器
func (s *ReservoirSampler[T]) Reset() {
	s.count = 0
	s.samples = s.samples[:0]
}
