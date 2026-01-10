package algorithm

import (
	"errors"
	"math/bits"
	"runtime"
	"sync/atomic"
)

// RingBuffer 是一个高性能、并发安全的 MPMC (Multi-Producer Multi-Consumer) 无锁队列。
// 它通过序列号 (Sequence) 解决槽位竞争与数据可见性问题，并采用缓存行填充防止伪共享。
type RingBuffer[T any] struct {
	slots []rbSlot[T]
	_     [40]byte // Padding to isolate head from other fields.
	head  uint64   // 消费者索引.
	_     [56]byte // Padding to isolate tail.
	tail  uint64   // 生产者索引.
	_     [56]byte // Padding to isolate mask.
	mask  uint64
}

// rbSlot 包装了队列中的元素和序列.
type rbSlot[T any] struct {
	data     T
	sequence uint64
}

var (
	ErrBufferFull  = errors.New("ring buffer is full")
	ErrBufferEmpty = errors.New("ring buffer is empty")
)

// NewRingBuffer 创建一个具有给定容量的新 RingBuffer。
// 容量必须是 2 的幂。
func NewRingBuffer[T any](capacity uint64) (*RingBuffer[T], error) {
	if capacity < 2 {
		return nil, errors.New("capacity must be at least 2")
	}

	// 确保 capacity 是 2 的.
	if (capacity & (capacity - 1)) != 0 {
		// 向上取整到最近的 2 的.
		// 例如：3 -> 4, 5 -> .
		capacity = 1 << (64 - bits.LeadingZeros64(capacity-1))
	}

	rb := &RingBuffer[T]{
		mask:  capacity - 1,
		slots: make([]rbSlot[T], capacity),
	}

	// 初始化序列号，sequence 初始值必须等于索引.
	// 这样第一次 Offer 时，sequence == tail (0) 检查才会通.
	for i := range capacity {
		rb.slots[i].sequence = i
	}

	return rb, nil
}

// Offer 向缓冲区添加一个项目 (Thread-safe for Multiple Producers)。
func (rb *RingBuffer[T]) Offer(item T) error {
	var slot *rbSlot[T]
	var tail uint64

	for {
		tail = atomic.LoadUint64(&rb.tail)
		slot = &rb.slots[tail&rb.mask]
		seq := atomic.LoadUint64(&slot.sequence)

		diff := int64(seq) - int64(tail)

		if diff == 0 {
			// sequence == tail 说明该 slot 空闲且已准备好被当前轮次的 tail 写.
			if atomic.CompareAndSwapUint64(&rb.tail, tail, tail+1) {
				break // 成功抢占 tai.
			}
		} else if diff < 0 {
			// sequence < tail 说明 slot 被上一轮占用且未释放，或 sequence 回绕（极少见.
			// 队列.
			return ErrBufferFull
		}
		runtime.Gosched()
	}

	slot.data = item
	// 将 sequence 更新为 tail + 1，表示数据已写入，消费者可.
	atomic.StoreUint64(&slot.sequence, tail+1)
	return nil
}

// Poll 从缓冲区取出一个项目 (Thread-safe for Multiple Consumers)。
func (rb *RingBuffer[T]) Poll() (T, error) {
	var slot *rbSlot[T]
	var head uint64
	var item T

	for {
		head = atomic.LoadUint64(&rb.head)
		slot = &rb.slots[head&rb.mask]
		seq := atomic.LoadUint64(&slot.sequence)

		diff := int64(seq) - int64(head+1)

		if diff == 0 {
			// sequence == head + 1 说明该 slot 已被生产者写入完成（Offer 中设置了 tail + 1.
			if atomic.CompareAndSwapUint64(&rb.head, head, head+1) {
				break // 成功抢占 hea.
			}
		} else if diff < 0 {
			// sequence < head + 1 说明 slot 数据尚未准备好（生产者正在写或队列空.
			return item, ErrBufferEmpty
		}
		runtime.Gosched()
	}

	item = slot.data
	// 重置 data 避免内存泄漏（对于指针类型.
	var zero T
	slot.data = zero
	// 将 sequence 更新为 head + capacity，表示该 slot 已空闲，可供下一轮生产者使.
	atomic.StoreUint64(&slot.sequence, head+uint64(len(rb.slots)))
	return item, nil
}

// Capacity 返回缓冲区总容量。
func (rb *RingBuffer[T]) Capacity() uint64 {
	return uint64(len(rb.slots))
}

// Len 返回当前缓冲区中的元素数量估计值。
func (rb *RingBuffer[T]) Len() uint64 {
	// 注意：由于是并发环境，head 和 tail 可能在读取瞬间不一致.
	// 此长度仅为近似值。
	tail := atomic.LoadUint64(&rb.tail)
	head := atomic.LoadUint64(&rb.head)
	if tail < head {
		return 0
	}
	return tail - head
}
