// Package structures 提供了高性能数据结构.
package structures

import (
	"log/slog"
	"math"
	"sync"

	algomath "github.com/wyfcoding/pkg/algorithm/math"
	"github.com/wyfcoding/pkg/xerrors"
)

const (
	bitsPerWord = 64
	wordShift   = 6
	wordMask    = 63
)

// BloomFilter 封装了高空间利用率的概率型数据结构.
type BloomFilter struct {
	bits   []uint64
	size   uint
	hashes uint
	mu     sync.RWMutex
}

// NewBloomFilter 根据预估容量与允许的误报率，科学计算并初始化布隆过滤器.
func NewBloomFilter(expectedElements uint, falsePositiveRate float64) (*BloomFilter, error) {
	if expectedElements == 0 {
		return nil, xerrors.ErrCapacityTooSmall // 使用中心化错误
	}

	if falsePositiveRate <= 0 || falsePositiveRate >= 1 {
		return nil, xerrors.ErrInvalidProbabilities
	}

	ln2 := math.Log(2)
	m := uint(math.Ceil(-float64(expectedElements) * math.Log(falsePositiveRate) / (ln2 * ln2)))
	k := uint(math.Ceil(float64(m) / float64(expectedElements) * ln2))

	slog.Info("bloom_filter initialized",
		"expected_elements", expectedElements,
		"false_positive_rate", falsePositiveRate,
		"bits_size", m,
		"hashes_count", k)

	return &BloomFilter{
		bits:   make([]uint64, (m+bitsPerWord-1)/bitsPerWord),
		size:   m,
		hashes: k,
		mu:     sync.RWMutex{},
	}, nil
}

// Add 向布隆过滤器中插入新的数据.
func (bf *BloomFilter) Add(data []byte) {
	bf.mu.Lock()
	defer bf.mu.Unlock()

	h1, h2 := bf.getHashes(data)
	for i := range bf.hashes {
		idx := (h1 + i*h2) % bf.size
		bf.bits[idx>>wordShift] |= (1 << (idx & wordMask))
	}
}

// Contains 执行成员存在性检查.
func (bf *BloomFilter) Contains(data []byte) bool {
	bf.mu.RLock()
	defer bf.mu.RUnlock()

	h1, h2 := bf.getHashes(data)
	for i := range bf.hashes {
		idx := (h1 + i*h2) % bf.size
		if bf.bits[idx>>wordShift]&(1<<(idx&wordMask)) == 0 {
			return false
		}
	}

	return true
}

func (bf *BloomFilter) getHashes(data []byte) (h1, h2 uint) {
	var h uint64 = algomath.FnvOffset64
	for _, b := range data {
		h ^= uint64(b)
		h *= algomath.FnvPrime64
	}

	return uint(h >> 32), uint(h)
}
