package structures

import (
	"runtime"
	"sync/atomic"

	"github.com/wyfcoding/pkg/cast"
)

// LockFreeQueue 是一个高性能、无锁、固定大小的 MPMC（多生产者多消费者）环形队列。
type LockFreeQueue[T any] struct {
	slots    []slot[T]
	capacity uint32
	mask     uint32
	_        [56]byte
	head     uint32
	_        [60]byte
	tail     uint32
	_        [60]byte
}

type slot[T any] struct {
	item     T
	_        [40]byte
	sequence uint32
}

// NewLockFreeQueue 创建一个指定容量的无锁队列。
func NewLockFreeQueue[T any](capacity uint32) *LockFreeQueue[T] {
	if capacity&(capacity-1) != 0 {
		shift := cast.IntToUint(32 - countLeadingZeros(capacity-1)&0x1F)
		capacity = 1 << shift
	}

	q := &LockFreeQueue[T]{
		capacity: capacity,
		mask:     capacity - 1,
		slots:    make([]slot[T], capacity),
	}

	for i := range capacity {
		q.slots[i].sequence = i
	}

	return q
}

// Push 将一个元素推入队列。如果队列已满，则返回 false。
func (q *LockFreeQueue[T]) Push(item T) bool {
	var s *slot[T]
	pos := atomic.LoadUint32(&q.tail)

	for {
		s = &q.slots[pos&q.mask]
		seq := atomic.LoadUint32(&s.sequence)
		dist := seq - pos
		switch {
		case dist == 0:
			if atomic.CompareAndSwapUint32(&q.tail, pos, pos+1) {
				goto breakOuter
			}
		case dist > 2147483647:
			return false
		default:
			pos = atomic.LoadUint32(&q.tail)
		}
		runtime.Gosched()
	}
breakOuter:

	s.item = item
	atomic.StoreUint32(&s.sequence, pos+1)
	return true
}

// Pop 从队列中弹出一个元素。如果队列为空，则返回 nil, false。
func (q *LockFreeQueue[T]) Pop() (T, bool) {
	var s *slot[T]
	pos := atomic.LoadUint32(&q.head)

	for {
		s = &q.slots[pos&q.mask]
		seq := atomic.LoadUint32(&s.sequence)
		dist := seq - (pos + 1)
		switch {
		case dist == 0:
			if atomic.CompareAndSwapUint32(&q.head, pos, pos+1) {
				goto breakOuterPop
			}
		case dist > 2147483647:
			var zero T
			return zero, false
		default:
			pos = atomic.LoadUint32(&q.head)
		}
		runtime.Gosched()
	}
breakOuterPop:

	item := s.item
	var zero T
	s.item = zero
	atomic.StoreUint32(&s.sequence, pos+q.mask+1)
	return item, true
}

// countLeadingZeros 计算前导零的数量。
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