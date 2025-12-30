package algorithm

import (
	"hash/fnv"
	"math/rand"
)

// CuckooFilter 工业级布谷鸟过滤器
// 支持插入、查询和删除操作
type CuckooFilter struct {
	buckets []bucket
	count   uint
	size    uint // 桶的数量
}

const (
	bucketSize = 4 // 每个桶存放 4 个指纹
	maxKicks   = 500
)

type bucket [bucketSize]byte

// NewCuckooFilter 创建布谷鸟过滤器
// capacity: 期望存放的元素总数
func NewCuckooFilter(capacity uint) *CuckooFilter {
	size := nextPowerOfTwo(capacity / bucketSize)
	if size == 0 {
		size = 1
	}
	return &CuckooFilter{
		buckets: make([]bucket, size),
		size:    size,
	}
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
	if rand.Intn(2) == 0 {
		i = i2
	}

	for range maxKicks {
		slot := rand.Intn(bucketSize)
		// 踢出旧指纹，换入新指纹
		oldFp := cf.buckets[i][slot]
		cf.buckets[i][slot] = fp
		fp = oldFp

		// 计算被踢出指纹的另一个候选桶位置
		// 利用 XOR 运算：i2 = i1 ^ hash(fp)
		i = i ^ cf.getHashOfFingerprint(fp)
		if cf.insertToBucket(i, fp) {
			cf.count++
			return true
		}
	}

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

func (cf *CuckooFilter) hash(data []byte) (uint, uint, byte) {
	h := fnv.New64a()
	h.Write(data)
	s := h.Sum64()

	i1 := uint(s) % cf.size
	fp := byte((s>>32)%255) + 1 // 保证指纹不为 0
	i2 := i1 ^ cf.getHashOfFingerprint(fp)

	return i1, i2 % cf.size, fp
}

func (cf *CuckooFilter) getHashOfFingerprint(fp byte) uint {
	// 使用指纹的哈希值进行 XOR 偏移
	h := fnv.New64a()
	h.Write([]byte{fp})
	return uint(h.Sum64())
}

func (cf *CuckooFilter) insertToBucket(i uint, fp byte) bool {
	for j := range bucketSize {
		if cf.buckets[i][j] == 0 {
			cf.buckets[i][j] = fp
			return true
		}
	}
	return false
}

func (cf *CuckooFilter) lookupInBucket(i uint, fp byte) bool {
	for j := range bucketSize {
		if cf.buckets[i][j] == fp {
			return true
		}
	}
	return false
}

func (cf *CuckooFilter) deleteFromBucket(i uint, fp byte) bool {
	for j := range bucketSize {
		if cf.buckets[i][j] == fp {
			cf.buckets[i][j] = 0
			return true
		}
	}
	return false
}

func nextPowerOfTwo(n uint) uint {
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n++
	return n
}
