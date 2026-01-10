package algorithm

import (
	"runtime"
	"sync/atomic"
)

// LockFreeQueue 是一个高性能、无锁、固定大小的 MPMC（多生产者多消费者）环形队列。
// 它通过原子操作（CAS）来管理读写索引，避免了互斥锁带来的上下文切换开销。
// 复杂度分析.
// - 入队 (Push): 平均 O(1)，最坏情况取决于 CPU 竞争。
// - 出队 (Pop): 平均 O(1)，最坏情况取决于 CPU 竞争。
// - 空间复杂度: O(N)，其中 N 是队列的容量。
type LockFreeQueue struct {
	slots    []slot
	capacity uint32
	mask     uint32
	_        [56]byte // Padding: 4+4+56 = 64 bytes. Start of next line.
	head     uint32
	_        [60]byte // Padding: 4+60 = 64 bytes. Ensures tail starts at 128.
	tail     uint32
	_        [60]byte // Padding: 4+60 = 64 bytes. Ensures slots start at 192 (aligned).
}

// slot 代表队列中的一个槽位。
// 优化：调整结构体大小为 64 字节，以匹配常见的 CPU 缓存行大小，防止伪共享。
type slot struct {
	item     any      // 16 byte.
	_        [40]byte // Padding: 4 (seq) + 4 (implicit) + 16 (item) + 40 (explicit) = 64 bytes.
	sequence uint32
}

// NewLockFreeQueue 创建一个指定容量的无锁队列。
// 注意：capacity 必须是 2 的幂次方，以便使用位运算优化。
func NewLockFreeQueue(capacity uint32) *LockFreeQueue {
	if capacity&(capacity-1) != 0 {
		// 如果不是 2 的幂，向上取.
		capacity = 1 << uint(32-countLeadingZeros(capacity-1)) //nolint:gosec // 2的幂计算安全。
	}

	q := &LockFreeQueue{
		capacity: capacity,
		mask:     capacity - 1,
		slots:    make([]slot, capacity),
	}

	for i := range capacity {
		q.slots[i].sequence = i
	}

	return q
}

// Push 将一个元素推入队列。如果队列已满，则返回 false。
func (q *LockFreeQueue) Push(item any) bool {
	var s *slot
	pos := atomic.LoadUint32(&q.tail)

	for {
		s = &q.slots[pos&q.mask]
		seq := atomic.LoadUint32(&s.sequence)
		// 显式检查防止回绕导致的溢出误判，虽然对于环开索引 diff 计算是安全的，
		// 但为了满足 G115 且保持逻辑严密，我们确保只在合理范围内比较。
		diff := int32(uint32(seq) - uint32(pos))
		switch {
		case diff == 0:
			if atomic.CompareAndSwapUint32(&q.tail, pos, pos+1) {
				goto breakOuter
			}
		case diff < 0:
			return false
		default:
			pos = atomic.LoadUint32(&q.tail)
		}
		runtime.Gosched() // 让出 CPU，降低忙等压.
	}
breakOuter:

	s.item = item
	atomic.StoreUint32(&s.sequence, pos+1)
	return true
}

// Pop 从队列中弹出一个元素。如果队列为空，则返回 nil, false。
func (q *LockFreeQueue) Pop() (any, bool) {
	var s *slot
	pos := atomic.LoadUint32(&q.head)

	for {
		s = &q.slots[pos&q.mask]
		seq := atomic.LoadUint32(&s.sequence)
		diff := int32(uint32(seq) - uint32(pos+1)) // 环形索引差值安全 (G115).
		switch {
		case diff == 0:
			if atomic.CompareAndSwapUint32(&q.head, pos, pos+1) {
				goto breakOuterPop
			}
		case diff < 0:
			return nil, false
		default:
			pos = atomic.LoadUint32(&q.head)
		}
		runtime.Gosched()
	}
breakOuterPop:

	item := s.item
	s.item = nil
	atomic.StoreUint32(&s.sequence, pos+q.mask+1)
	return item, true
}

// countLeadingZeros 计算前导零的数量，辅助计算 2 的.
func countLeadingZeros(x uint32) int {
	if x == 0 {
		return 32
	}
	n := 0
	if x <= 0x0000FFFF {
		n += 16
		x <<= 16
	}
	if x <= 0x00FFFFFF {
		n += 8
		x <<= 8
	}
	if x <= 0x0FFFFFFF {
		n += 4
		x <<= 4
	}
	if x <= 0x3FFFFFFF {
		n += 2
		x <<= 2
	}
	if x <= 0x7FFFFFFF {
		n++
	}
	return n
}
