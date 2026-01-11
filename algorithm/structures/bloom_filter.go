// Package structures 提供了高性能数据结构.
package structures

import (
	"log/slog"
	"math"
	"sync/atomic"

	algomath "github.com/wyfcoding/pkg/algorithm/math"
	"github.com/wyfcoding/pkg/utils"
	"github.com/wyfcoding/pkg/xerrors"
)

const (
	bitsPerWord = 64
	wordShift   = 6
	wordMask    = 63
)

// BloomFilter 封装了高空间利用率的概率型数据结构.
type BloomFilter struct {
	bits []uint64
	size uint
	hash uint
}

// NewBloomFilter 根据预估容量与允许的误报率，科学计算并初始化布隆过滤器.
func NewBloomFilter(expectedElements uint, falsePositiveRate float64) (*BloomFilter, error) {
	if expectedElements == 0 {
		return nil, xerrors.ErrCapacityTooSmall
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
		bits: make([]uint64, (m+bitsPerWord-1)/bitsPerWord),
		size: m,
		hash: k,
	}, nil
}

// Add 向布隆过滤器中插入新的数据.
// 优化：使用原子操作实现无锁化 Add。
func (bf *BloomFilter) Add(data []byte) {
	h1, h2 := bf.getHashes(data)
	for i := range bf.hash {
		// 双哈希模拟 k 个哈希函数。
		idx := (uint64(h1) + uint64(i)*uint64(h2)) % uint64(bf.size)
		wordIdx := idx >> wordShift
		bitMask := uint64(1) << (idx & wordMask)

		// 原子 OR 操作，确保并发安全且无锁。
		for {
			old := atomic.LoadUint64(&bf.bits[wordIdx])
			if old&bitMask != 0 || atomic.CompareAndSwapUint64(&bf.bits[wordIdx], old, old|bitMask) {
				break
			}
		}
	}
}

// Contains 执行成员存在性检查.
// 优化：无锁读取。
func (bf *BloomFilter) Contains(data []byte) bool {
	h1, h2 := bf.getHashes(data)
	for i := range bf.hash {
		idx := (uint64(h1) + uint64(i)*uint64(h2)) % uint64(bf.size)
		if (atomic.LoadUint64(&bf.bits[idx>>wordShift]) & (1 << (idx & wordMask))) == 0 {
			return false
		}
	}

	return true
}

func (bf *BloomFilter) getHashes(data []byte) (h1, h2 uint32) {
	var h uint64 = algomath.FnvOffset64
	for _, b := range data {
		h ^= uint64(b)
		h *= algomath.FnvPrime64
	}

	return utils.Uint64ToUint32(h >> 32), utils.Uint64ToUint32(h)
}
