package structures

import (
	"crypto/rand"
	"log/slog"
	"math/bits"

	algomath "github.com/wyfcoding/pkg/algorithm/math"
	"github.com/wyfcoding/pkg/xerrors"
)

const (
	bucketSize = 4
	maxKicks   = 500
	fpModulo   = 255
)

type bucket [bucketSize]byte

// CuckooFilter 工业级布谷鸟过滤器.
type CuckooFilter struct {
	buckets []bucket
	count   uint
	size    uint
	mask    uint
}

// NewCuckooFilter 创建布谷鸟过滤器.
func NewCuckooFilter(capacity uint) (*CuckooFilter, error) {
	if capacity == 0 {
		return nil, xerrors.ErrCapacityRequired
	}

	n := (capacity + bucketSize - 1) / bucketSize
	if n == 0 {
		n = 1
	}

	if n&(n-1) != 0 {
		n = 1 << (64 - bits.LeadingZeros64(uint64(n-1)))
	}

	slog.Info("CuckooFilter initialized", "capacity", capacity, "buckets_size", n)

	return &CuckooFilter{
		buckets: make([]bucket, n),
		size:    n,
		mask:    n - 1,
		count:   0,
	}, nil
}

// Add 插入元素.
func (cf *CuckooFilter) Add(data []byte) bool {
	idx1, idx2, fp := cf.getHashes(data)

	if cf.insertToBucket(idx1, fp) || cf.insertToBucket(idx2, fp) {
		cf.count++

		return true
	}

	currIdx := idx1
	var b [1]byte
	_, _ = rand.Read(b[:])
	if b[0]%2 == 0 {
		currIdx = idx2
	}

	for range maxKicks {
		var b2 [1]byte
		_, _ = rand.Read(b2[:])
		slot := int(b2[0]) % bucketSize
		cf.buckets[currIdx][slot], fp = fp, cf.buckets[currIdx][slot]

		currIdx = (currIdx ^ cf.getHashOfFingerprint(fp)) & cf.mask

		if cf.insertToBucket(currIdx, fp) {
			cf.count++

			return true
		}
	}

	slog.Warn("CuckooFilter is full, insertion failed", "count", cf.count, "buckets", cf.size)

	return false
}

// Contains 查询元素是否存在.
func (cf *CuckooFilter) Contains(data []byte) bool {
	idx1, idx2, fp := cf.getHashes(data)

	return cf.lookupInBucket(idx1, fp) || cf.lookupInBucket(idx2, fp)
}

// Delete 删除元素.
func (cf *CuckooFilter) Delete(data []byte) bool {
	idx1, idx2, fp := cf.getHashes(data)
	if cf.deleteFromBucket(idx1, fp) || cf.deleteFromBucket(idx2, fp) {
		cf.count--

		return true
	}

	return false
}

func (cf *CuckooFilter) getHashes(data []byte) (h1, h2 uint, fp byte) {
	var h uint64 = algomath.FnvOffset64
	for _, b := range data {
		h ^= uint64(b)
		h *= algomath.FnvPrime64
	}

	idx1 := uint(h) & cf.mask
	fp = byte((h>>32)%fpModulo) + 1
	idx2 := (idx1 ^ cf.getHashOfFingerprint(fp)) & cf.mask

	return idx1, idx2, fp
}

func (cf *CuckooFilter) getHashOfFingerprint(fp byte) uint {
	var h uint64 = algomath.FnvOffset64
	h ^= uint64(fp)
	h *= algomath.FnvPrime64

	return uint(h)
}

func (cf *CuckooFilter) insertToBucket(i uint, fp byte) bool {
	b := &cf.buckets[i]
	for idx := range bucketSize {
		if b[idx] == 0 {
			b[idx] = fp

			return true
		}
	}

	return false
}

func (cf *CuckooFilter) lookupInBucket(i uint, fp byte) bool {
	b := &cf.buckets[i]
	for idx := range bucketSize {
		if b[idx] == fp {
			return true
		}
	}

	return false
}

func (cf *CuckooFilter) deleteFromBucket(i uint, fp byte) bool {
	b := &cf.buckets[i]
	for idx := range bucketSize {
		if b[idx] == fp {
			b[idx] = 0

			return true
		}
	}

	return false
}
