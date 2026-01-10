package algorithm

import (
	"errors"
	"log/slog"
	"math/bits"
	"math/rand/v2"
)

// CuckooFilter 工业级布谷鸟过滤器
// 支持插入、查询和删除操作
type CuckooFilter struct {
	buckets []bucket
	count   uint
	size    uint // 桶的数量
	mask    uint // size - 1 (Assuming size is power of 2)
}

const (
	bucketSize = 4 // 每个桶存放 4 个指纹
	maxKicks   = 500
)

type bucket [bucketSize]byte

// NewCuckooFilter 创建布谷鸟过滤器
// capacity: 期望存放的元素总数
func NewCuckooFilter(capacity uint) (*CuckooFilter, error) {
	if capacity == 0 {
		return nil, errors.New("capacity must be greater than 0")
	}
	// 容量/4，然后向上取整到 2 的幂
	n := (capacity + bucketSize - 1) / bucketSize
	if n == 0 {
		n = 1
	}
	// next power of 2
	if n&(n-1) != 0 {
		n = 1 << (64 - bits.LeadingZeros64(uint64(n-1)))
	}

	slog.Info("CuckooFilter initialized", "capacity", capacity, "buckets_size", n)
	return &CuckooFilter{
		buckets: make([]bucket, n),
		size:    n,
		mask:    n - 1,
	}, nil
}

// Add 插入元素
func (cf *CuckooFilter) Add(data []byte) bool {
	i1, i2, fp := cf.hash(data)

	// 尝试直接插入第一个或第二个候选桶
	if cf.insertToBucket(i1, fp) || cf.insertToBucket(i2, fp) {
		cf.count++
		return true
	}

	// 桶满了，执行“踢出（Relocation）”逻辑
	i := i1
	if rand.IntN(2) == 0 {
		i = i2
	}

	for range maxKicks {
		slot := rand.IntN(bucketSize)
		// 踢出旧指纹，换入新指纹
		oldFp := cf.buckets[i][slot]
		cf.buckets[i][slot] = fp
		fp = oldFp

		// 计算被踢出指纹的另一个候选桶位置
		// i2 = i1 ^ hash(fp)
		// 注意：这里需要确保 hash(fp) 与 i 取模后的结果一致
		// 由于 size 是 2 的幂，我们使用 & mask
		i = (i ^ cf.getHashOfFingerprint(fp)) & cf.mask

		if cf.insertToBucket(i, fp) {
			cf.count++
			return true
		}
	}

	slog.Warn("CuckooFilter is full, insertion failed", "count", cf.count, "buckets", cf.size)
	return false // 过滤器太满，插入失败
}

// Contains 查询元素是否存在
func (cf *CuckooFilter) Contains(data []byte) bool {
	i1, i2, fp := cf.hash(data)
	return cf.lookupInBucket(i1, fp) || cf.lookupInBucket(i2, fp)
}

// Delete 删除元素 (布隆过滤器做不到的特性)
func (cf *CuckooFilter) Delete(data []byte) bool {
	i1, i2, fp := cf.hash(data)
	if cf.deleteFromBucket(i1, fp) || cf.deleteFromBucket(i2, fp) {
		cf.count--
		return true
	}
	return false
}

// --- 内部辅助函数 ---

// hashReturns i1, i2, fingerprint
func (cf *CuckooFilter) hash(data []byte) (uint, uint, byte) {
	// FNV-1a 64-bit inline
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	var h uint64 = offset64
	for _, b := range data {
		h ^= uint64(b)
		h *= prime64
	}

	i1 := uint(h) & cf.mask
	fp := byte((h>>32)%255) + 1 // 保证指纹不为 0
	i2 := (i1 ^ cf.getHashOfFingerprint(fp)) & cf.mask

	return i1, i2, fp
}

func (cf *CuckooFilter) getHashOfFingerprint(fp byte) uint {
	// 简单混淆指纹生成偏移量
	// FNV-1a with single byte
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	var h uint64 = offset64
	h ^= uint64(fp)
	h *= prime64
	return uint(h)
}

func (cf *CuckooFilter) insertToBucket(i uint, fp byte) bool {
	// Unroll loop for small bucketSize=4
	b := &cf.buckets[i]
	if b[0] == 0 {
		b[0] = fp
		return true
	}
	if b[1] == 0 {
		b[1] = fp
		return true
	}
	if b[2] == 0 {
		b[2] = fp
		return true
	}
	if b[3] == 0 {
		b[3] = fp
		return true
	}
	return false
}

func (cf *CuckooFilter) lookupInBucket(i uint, fp byte) bool {
	b := &cf.buckets[i]
	return b[0] == fp || b[1] == fp || b[2] == fp || b[3] == fp
}

func (cf *CuckooFilter) deleteFromBucket(i uint, fp byte) bool {
	b := &cf.buckets[i]
	if b[0] == fp {
		b[0] = 0
		return true
	}
	if b[1] == fp {
		b[1] = 0
		return true
	}
	if b[2] == fp {
		b[2] = 0
		return true
	}
	if b[3] == fp {
		b[3] = 0
		return true
	}
	return false
}
